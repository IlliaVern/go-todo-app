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

func validateTitle(title string) error {
	if strings.TrimSpace(title) == "" {
		return ErrTitleRequired
	}
	if utf8.RuneCountInString(title) > MaxTitleLength {
		return ErrTitleTooLong
	}
	if containsNUL(title) {
		return ErrNULCharacter
	}
	return nil
}

func validateDescription(description string) error {
	if utf8.RuneCountInString(description) > MaxDescriptionLength {
		return ErrDescriptionTooLong
	}
	if containsNUL(description) {
		return ErrNULCharacter
	}
	return nil
}

// containsNUL reports a character PostgreSQL text columns cannot store.
func containsNUL(s string) bool { return strings.ContainsRune(s, 0) }
