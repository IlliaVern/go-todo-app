// Package domain is the identity bounded context's model: registered users,
// their credential rules and the user repository port.
package domain

import (
	"context"
	"errors"
	"fmt"
)

const (
	// MaxNameLength and MaxUsernameLength mirror the varchar(255) columns in schema/.
	MaxNameLength     = 255
	MaxUsernameLength = 255
	// MaxPasswordBytes bounds hashing input; argon2id has no length limit of its own.
	MaxPasswordBytes = 1024
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
	ErrNameRequired       = &ValidationError{"name is required"}
	ErrNameTooLong        = &ValidationError{fmt.Sprintf("name must be at most %d characters", MaxNameLength)}
	ErrUsernameRequired   = &ValidationError{"username is required"}
	ErrUsernameTooLong    = &ValidationError{fmt.Sprintf("username must be at most %d characters", MaxUsernameLength)}
	ErrUsernameWhitespace = &ValidationError{"username must not start or end with whitespace"}
	ErrPasswordRequired   = &ValidationError{"password is required"}
	ErrPasswordTooLong    = &ValidationError{fmt.Sprintf("password must be at most %d bytes", MaxPasswordBytes)}
	ErrNULCharacter       = &ValidationError{"name and username must not contain NUL characters"}
)

var ErrPasswordHashEmpty = errors.New("password hash is empty")

// User is an aggregate root: a registered account. It holds the password
// hash only, never the plain password.
type User struct {
	id           UserID
	name         Name
	username     Username
	passwordHash PasswordHash
}

func NewUser(name Name, username Username, passwordHash PasswordHash) (User, error) {
	if passwordHash == "" {
		return User{}, ErrPasswordHashEmpty
	}
	return User{name: name, username: username, passwordHash: passwordHash}, nil
}

// RestoreUser rebuilds a persisted user from trusted storage without
// re-validating it. Only repositories call it.
func RestoreUser(id UserID, name, username string, passwordHash PasswordHash) User {
	return User{id: id, name: Name{name}, username: Username{username}, passwordHash: passwordHash}
}

func (u User) ID() UserID                 { return u.id }
func (u User) Name() Name                 { return u.name }
func (u User) Username() Username         { return u.username }
func (u User) PasswordHash() PasswordHash { return u.passwordHash }

type UserRepository interface {
	// Add stores a new user and returns its id, or ErrUsernameTaken.
	Add(ctx context.Context, user User) (UserID, error)
	// ByUsername returns the user or ErrUserNotFound.
	ByUsername(ctx context.Context, username Username) (User, error)
	Exists(ctx context.Context, id UserID) (bool, error)
}
