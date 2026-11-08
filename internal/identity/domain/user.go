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
	ErrUserNotFound     = errors.New("user not found")
	ErrUsernameTaken    = errors.New("username is already taken")
	ErrConcurrentUpdate = errors.New("user was modified concurrently")
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
	ErrEmptyUpdate        = &ValidationError{"update structure has no values"}
)

var ErrPasswordHashEmpty = errors.New("password hash is empty")

// User is an aggregate root: a registered account. It holds the password
// hash only, never the plain password.
type User struct {
	id           UserID
	name         Name
	username     Username
	passwordHash PasswordHash
	// version increases on every saved update; the repository uses it for
	// optimistic concurrency control.
	version int64
}

func NewUser(name Name, username Username, passwordHash PasswordHash) (User, error) {
	if passwordHash == "" {
		return User{}, ErrPasswordHashEmpty
	}
	return User{name: name, username: username, passwordHash: passwordHash}, nil
}

// RestoreUser rebuilds a persisted user from trusted storage without
// re-validating it. Only repositories call it.
func RestoreUser(id UserID, name, username string, passwordHash PasswordHash, version int64) User {
	return User{id: id, name: Name{name}, username: Username{username}, passwordHash: passwordHash, version: version}
}

func (u User) ID() UserID                 { return u.id }
func (u User) Name() Name                 { return u.name }
func (u User) Username() Username         { return u.username }
func (u User) PasswordHash() PasswordHash { return u.passwordHash }
func (u User) Version() int64             { return u.version }

// ProfilePatch is a partial update; nil fields stay unchanged.
type ProfilePatch struct {
	Name     *string
	Username *string
}

// Apply updates the profile from p, or leaves it untouched and returns a
// ValidationError when p is empty or a value breaks an invariant.
func (u *User) Apply(p ProfilePatch) error {
	if p.Name == nil && p.Username == nil {
		return ErrEmptyUpdate
	}

	name, username := u.name, u.username
	var err error
	if p.Name != nil {
		if name, err = NewName(*p.Name); err != nil {
			return err
		}
	}
	if p.Username != nil {
		if username, err = NewUsername(*p.Username); err != nil {
			return err
		}
	}
	u.name, u.username = name, username
	return nil
}

func (u *User) SetPasswordHash(hash PasswordHash) error {
	if hash == "" {
		return ErrPasswordHashEmpty
	}
	u.passwordHash = hash
	return nil
}

type UserRepository interface {
	// Add stores a new user and returns its id, or ErrUsernameTaken.
	Add(ctx context.Context, user User) (UserID, error)
	// Get returns the user or ErrUserNotFound.
	Get(ctx context.Context, id UserID) (User, error)
	// ByUsername returns the user or ErrUserNotFound.
	ByUsername(ctx context.Context, username Username) (User, error)
	Exists(ctx context.Context, id UserID) (bool, error)
	// Update saves user if its version is still current. It returns
	// ErrConcurrentUpdate when another writer got there first, ErrUsernameTaken
	// or ErrUserNotFound.
	Update(ctx context.Context, user User) error
	// Delete removes the user and everything they own, or returns ErrUserNotFound.
	Delete(ctx context.Context, id UserID) error
}
