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
	Hash(ctx context.Context, password domain.Password) (domain.PasswordHash, error)
	Matches(ctx context.Context, hash domain.PasswordHash, password domain.Password) (bool, error)
}

type TokenManager interface {
	Issue(userID domain.UserID) (domain.AccessToken, error)
	Verify(token domain.AccessToken) (domain.UserID, error)
}

type Service struct {
	users  domain.UserRepository
	hasher PasswordHasher
	tokens TokenManager
	// dummyHash is checked when the username is unknown, so a failed sign-in
	// takes the same time whether or not the user exists.
	dummyHash domain.PasswordHash
}

func NewService(users domain.UserRepository, hasher PasswordHasher, tokens TokenManager) (*Service, error) {
	placeholder, err := domain.NewPassword("timing-equalisation-placeholder")
	if err != nil {
		return nil, fmt.Errorf("build dummy password: %w", err)
	}
	dummyHash, err := hasher.Hash(context.Background(), placeholder)
	if err != nil {
		return nil, fmt.Errorf("hash dummy password: %w", err)
	}
	return &Service{users: users, hasher: hasher, tokens: tokens, dummyHash: dummyHash}, nil
}

// SignUpInput is the unvalidated registration form.
type SignUpInput struct {
	Name     string
	Username string
	Password string
}

func (s *Service) SignUp(ctx context.Context, in SignUpInput) (domain.UserID, error) {
	// Validate everything before paying for password hashing.
	name, err := domain.NewName(in.Name)
	if err != nil {
		return 0, err
	}
	username, err := domain.NewUsername(in.Username)
	if err != nil {
		return 0, err
	}
	password, err := domain.NewPassword(in.Password)
	if err != nil {
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

// SignInInput is the unvalidated sign-in form.
type SignInInput struct {
	Username string
	Password string
}

// SignIn checks the credentials and returns an access token, or ErrInvalidCredentials.
func (s *Service) SignIn(ctx context.Context, in SignInInput) (domain.AccessToken, error) {
	// No stored password is empty or oversized, and rejecting one before
	// hashing reveals nothing about the username.
	password, err := domain.NewPassword(in.Password)
	if err != nil {
		return "", ErrInvalidCredentials
	}
	// An invalid username cannot match a stored one.
	username, err := domain.NewUsername(in.Username)
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
func (s *Service) rejectAfterDummyCheck(ctx context.Context, password domain.Password) error {
	if _, err := s.hasher.Matches(ctx, s.dummyHash, password); err != nil {
		return fmt.Errorf("verify password: %w", err)
	}
	return ErrInvalidCredentials
}

// Authenticate returns the id of the still-registered user a valid token was
// issued to, or ErrInvalidToken.
func (s *Service) Authenticate(ctx context.Context, token domain.AccessToken) (domain.UserID, error) {
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
