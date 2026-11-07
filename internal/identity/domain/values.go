package domain

import (
	"strings"
	"unicode/utf8"
)

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
