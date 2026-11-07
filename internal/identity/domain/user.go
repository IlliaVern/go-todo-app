// Package domain is the identity bounded context's model: registered users,
// their credential rules and the user repository port.
package domain

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"unicode/utf8"
)

const (
	// MaxNameLength and MaxUsernameLength mirror the varchar(255) columns in schema/.
	MaxNameLength     = 255
	MaxUsernameLength = 255
	// MaxPasswordBytes is bcrypt's input limit.
	MaxPasswordBytes = 72
)

var (
	ErrUserNotFound  = errors.New("user not found")
	ErrUsernameTaken = errors.New("username is already taken")
)

// ValidationError reports user input that violates an identity invariant.
type ValidationError struct {
	msg string
}

func (e *ValidationError) Error() string { return e.msg }

var (
	ErrNameRequired      = &ValidationError{"name is required"}
	ErrNameTooLong       = &ValidationError{fmt.Sprintf("name must be at most %d characters", MaxNameLength)}
	ErrUsernameRequired  = &ValidationError{"username is required"}
	ErrUsernameTooLong   = &ValidationError{fmt.Sprintf("username must be at most %d characters", MaxUsernameLength)}
	ErrPasswordRequired  = &ValidationError{"password is required"}
	ErrPasswordTooLong   = &ValidationError{fmt.Sprintf("password must be at most %d bytes", MaxPasswordBytes)}
	ErrPasswordHashEmpty = errors.New("password hash is empty")
)

// User is an aggregate root: a registered account. It holds the password
// hash only, never the plain password.
type User struct {
	id           int64
	name         string
	username     string
	passwordHash string
}

// NewUser creates a user that has not been persisted yet.
func NewUser(name, username, passwordHash string) (User, error) {
	switch {
	case strings.TrimSpace(name) == "":
		return User{}, ErrNameRequired
	case utf8.RuneCountInString(name) > MaxNameLength:
		return User{}, ErrNameTooLong
	case strings.TrimSpace(username) == "":
		return User{}, ErrUsernameRequired
	case utf8.RuneCountInString(username) > MaxUsernameLength:
		return User{}, ErrUsernameTooLong
	case passwordHash == "":
		return User{}, ErrPasswordHashEmpty
	}
	return User{name: name, username: username, passwordHash: passwordHash}, nil
}

// RestoreUser rebuilds a persisted user. Only repositories call it.
func RestoreUser(id int64, name, username, passwordHash string) User {
	return User{id: id, name: name, username: username, passwordHash: passwordHash}
}

func (u User) ID() int64            { return u.id }
func (u User) Name() string         { return u.name }
func (u User) Username() string     { return u.username }
func (u User) PasswordHash() string { return u.passwordHash }

// ValidatePassword checks a plain password against the password policy.
func ValidatePassword(password string) error {
	switch {
	case password == "":
		return ErrPasswordRequired
	case len(password) > MaxPasswordBytes:
		return ErrPasswordTooLong
	}
	return nil
}

// UserRepository persists users.
type UserRepository interface {
	// Add stores a new user and returns its id, or ErrUsernameTaken.
	Add(ctx context.Context, user User) (int64, error)
	// ByUsername returns the user or ErrUserNotFound.
	ByUsername(ctx context.Context, username string) (User, error)
}
