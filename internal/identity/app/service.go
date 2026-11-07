// Package app holds the identity bounded context's use cases: signing up,
// signing in and authenticating access tokens.
package app

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
)

var (
	ErrInvalidCredentials = errors.New("invalid username or password")
	ErrInvalidToken       = errors.New("invalid or expired access token")
)

// PasswordHasher hashes and verifies passwords.
type PasswordHasher interface {
	Hash(password string) (string, error)
	// Matches reports whether password corresponds to hash.
	Matches(hash, password string) bool
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
	dummyHash, err := hasher.Hash("timing-equalisation-placeholder")
	if err != nil {
		return nil, fmt.Errorf("hash dummy password: %w", err)
	}
	return &Service{users: users, hasher: hasher, tokens: tokens, dummyHash: dummyHash}, nil
}

// SignUp registers a user and returns its id.
func (s *Service) SignUp(ctx context.Context, name, username, password string) (int64, error) {
	if err := domain.ValidateProfile(name, username); err != nil {
		return 0, err
	}
	if err := domain.ValidatePassword(password); err != nil {
		return 0, err
	}
	hash, err := s.hasher.Hash(password)
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
func (s *Service) SignIn(ctx context.Context, username, password string) (string, error) {
	// A NUL byte cannot match any stored username, and PostgreSQL rejects it.
	if strings.ContainsRune(username, 0) {
		s.hasher.Matches(s.dummyHash, password)
		return "", ErrInvalidCredentials
	}

	user, err := s.users.ByUsername(ctx, username)
	if errors.Is(err, domain.ErrUserNotFound) {
		s.hasher.Matches(s.dummyHash, password)
		return "", ErrInvalidCredentials
	}
	if err != nil {
		return "", err
	}

	if !s.hasher.Matches(user.PasswordHash(), password) {
		return "", ErrInvalidCredentials
	}

	return s.tokens.Issue(user.ID())
}

// Authenticate returns the user id carried by a valid token, or ErrInvalidToken.
func (s *Service) Authenticate(token string) (int64, error) {
	userID, err := s.tokens.Verify(token)
	if err != nil {
		return 0, fmt.Errorf("%w: %w", ErrInvalidToken, err)
	}
	return userID, nil
}
