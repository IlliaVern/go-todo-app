// Package app holds the identity bounded context's use cases: signing up,
// signing in and authenticating access tokens.
package app

import (
	"context"
	"errors"
	"fmt"

	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidToken       = errors.New("invalid or expired access token")
	// ErrHasherBusy is returned by PasswordHasher implementations that shed
	// load instead of queueing indefinitely.
	ErrHasherBusy = errors.New("server is busy, retry later")
)

// PasswordHasher hashes and verifies passwords. Both calls may wait for a
// free hashing slot and fail when ctx ends first.
type PasswordHasher interface {
	Hash(ctx context.Context, password string) (string, error)
	// Matches reports whether password corresponds to hash.
	Matches(ctx context.Context, hash, password string) (bool, error)
}

// TokenManager issues and verifies access tokens carrying a user id.
type TokenManager interface {
	Issue(userID int64) (string, error)
	Verify(token string) (int64, error)
}

type Service struct {
	users  domain.UserRepository
	hasher PasswordHasher
	tokens TokenManager
	// dummyHash is checked when the username is unknown, so a failed sign-in
	// takes the same time whether or not the user exists.
	dummyHash string
}

func NewService(users domain.UserRepository, hasher PasswordHasher, tokens TokenManager) (*Service, error) {
	dummyHash, err := hasher.Hash(context.Background(), "timing-equalisation-placeholder")
	if err != nil {
		return nil, fmt.Errorf("hash dummy password: %w", err)
	}
	return &Service{users: users, hasher: hasher, tokens: tokens, dummyHash: dummyHash}, nil
}

// SignUp registers a user and returns its id.
func (s *Service) SignUp(ctx context.Context, rawName, rawUsername, password string) (int64, error) {
	// Validate everything before paying for password hashing.
	name, err := domain.NewName(rawName)
	if err != nil {
		return 0, err
	}
	username, err := domain.NewUsername(rawUsername)
	if err != nil {
		return 0, err
	}
	if err := domain.ValidatePassword(password); err != nil {
		return 0, err
	}
	hash, err := s.hasher.Hash(ctx, password)
	if err != nil {
		return 0, fmt.Errorf("hash password: %w", err)
	}
	user, err := domain.NewUser(name, username, hash)
	if err != nil {
		return 0, err
	}
	return s.users.Add(ctx, user)
}

// SignIn checks the credentials and returns an access token, or ErrInvalidCredentials.
func (s *Service) SignIn(ctx context.Context, rawUsername, password string) (string, error) {
	// No stored password is this long, and rejecting it before hashing reveals
	// nothing about the username.
	if len(password) > domain.MaxPasswordBytes {
		return "", ErrInvalidCredentials
	}
	// An invalid username cannot match a stored one.
	username, err := domain.NewUsername(rawUsername)
	if err != nil {
		return "", s.rejectAfterDummyCheck(ctx, password)
	}

	user, err := s.users.ByUsername(ctx, username)
	if errors.Is(err, domain.ErrUserNotFound) {
		return "", s.rejectAfterDummyCheck(ctx, password)
	}
	if err != nil {
		return "", err
	}

	ok, err := s.hasher.Matches(ctx, user.PasswordHash(), password)
	if err != nil {
		return "", fmt.Errorf("verify password: %w", err)
	}
	if !ok {
		return "", ErrInvalidCredentials
	}

	return s.tokens.Issue(user.ID())
}

// rejectAfterDummyCheck spends the same hashing work as a real check, so
// unknown usernames cannot be told apart by timing.
func (s *Service) rejectAfterDummyCheck(ctx context.Context, password string) error {
	if _, err := s.hasher.Matches(ctx, s.dummyHash, password); err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	return ErrInvalidCredentials
}

// Authenticate returns the id of the still-registered user a valid token was
// issued to, or ErrInvalidToken.
func (s *Service) Authenticate(ctx context.Context, token string) (int64, error) {
	userID, err := s.tokens.Verify(token)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	exists, err := s.users.Exists(ctx, userID)
	if err != nil {
		return 0, fmt.Errorf("look up token user: %w", err)
	}
	if !exists {
		return 0, fmt.Errorf("%w: user %d no longer exists", ErrInvalidToken, userID)
	}
	return userID, nil
}
