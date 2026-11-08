package domain

import (
	"strings"
	"unicode/utf8"
)

// UserID identifies a registered user.
type UserID int64

// PasswordHash is the encoded result of hashing a password.
type PasswordHash string

// AccessToken is a signed credential proving who a request acts for.
type AccessToken string

// Name is a user's non-blank display name, at most MaxNameLength characters.
type Name struct{ value string }

func NewName(s string) (Name, error) {
	switch {
	case strings.TrimSpace(s) == "":
		return Name{}, ErrNameRequired
	case utf8.RuneCountInString(s) > MaxNameLength:
		return Name{}, ErrNameTooLong
	case strings.ContainsRune(s, 0):
		return Name{}, ErrNULCharacter
	}
	return Name{s}, nil
}

func (n Name) String() string { return n.value }

// Username is the unique login name: lowercase, at most MaxUsernameLength
// characters, without surrounding whitespace. "Ada" and "ada" are the same
// username.
type Username struct{ value string }

func NewUsername(s string) (Username, error) {
	switch {
	case strings.TrimSpace(s) == "":
		return Username{}, ErrUsernameRequired
	case strings.TrimSpace(s) != s:
		return Username{}, ErrUsernameWhitespace
	case utf8.RuneCountInString(s) > MaxUsernameLength:
		return Username{}, ErrUsernameTooLong
	case strings.ContainsRune(s, 0):
		return Username{}, ErrNULCharacter
	}
	return Username{strings.ToLower(s)}, nil
}

func (u Username) String() string { return u.value }

// Password is a non-empty plain password of at most MaxPasswordBytes.
// Formatting redacts it, so only an explicit Reveal can expose the secret.
type Password struct{ value string }

func NewPassword(s string) (Password, error) {
	switch {
	case s == "":
		return Password{}, ErrPasswordRequired
	case len(s) > MaxPasswordBytes:
		return Password{}, ErrPasswordTooLong
	}
	return Password{s}, nil
}

func (p Password) Reveal() string { return p.value }

func (Password) String() string { return "[redacted]" }

func (Password) GoString() string { return "[redacted]" }
