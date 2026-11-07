package domain

import (
	"strings"
	"unicode/utf8"
)

// Limits mirror the varchar(255) columns in schema/.
const (
	MaxTitleLength       = 255
	MaxDescriptionLength = 255
)

// Title is the non-blank title of a list or item, at most MaxTitleLength
// characters. Its zero value is never valid; build one with NewTitle.
type Title struct{ value string }

func NewTitle(s string) (Title, error) {
	switch {
	case strings.TrimSpace(s) == "":
		return Title{}, ErrTitleRequired
	case utf8.RuneCountInString(s) > MaxTitleLength:
		return Title{}, ErrTitleTooLong
	case containsNUL(s):
		return Title{}, ErrNULCharacter
	}
	return Title{s}, nil
}

func (t Title) String() string { return t.value }

// Description is the optional description of a list or item, at most
// MaxDescriptionLength characters; the zero value is the empty description.
type Description struct{ value string }

func NewDescription(s string) (Description, error) {
	switch {
	case utf8.RuneCountInString(s) > MaxDescriptionLength:
		return Description{}, ErrDescriptionTooLong
	case containsNUL(s):
		return Description{}, ErrNULCharacter
	}
	return Description{s}, nil
}

func (d Description) String() string { return d.value }

func ParseText(title, description string) (Title, Description, error) {
	t, err := NewTitle(title)
	if err != nil {
		return Title{}, Description{}, err
	}
	d, err := NewDescription(description)
	if err != nil {
		return Title{}, Description{}, err
	}
	return t, d, nil
}

// containsNUL reports a character PostgreSQL text columns cannot store.
func containsNUL(s string) bool { return strings.ContainsRune(s, 0) }
