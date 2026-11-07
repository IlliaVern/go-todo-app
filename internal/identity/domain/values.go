package domain

import (
	"strings"
	"unicode/utf8"
)

// Name is a user's non-blank display name, at most MaxNameLength characters.
type Name struct{ value string }

// NewName validates s as a display name.
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

// Username is the unique, non-blank login name, at most MaxUsernameLength
// characters.
type Username struct{ value string }

// NewUsername validates s as a username.
func NewUsername(s string) (Username, error) {
	switch {
	case strings.TrimSpace(s) == "":
		return Username{}, ErrUsernameRequired
	case utf8.RuneCountInString(s) > MaxUsernameLength:
		return Username{}, ErrUsernameTooLong
	case strings.ContainsRune(s, 0):
		return Username{}, ErrNULCharacter
	}
	return Username{s}, nil
}

func (u Username) String() string { return u.value }
