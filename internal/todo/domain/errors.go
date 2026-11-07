package domain

import (
	"errors"
	"fmt"
)

var (
	ErrListNotFound = errors.New("todo list not found")
	ErrItemNotFound = errors.New("todo item not found")
	// ErrOwnerNotFound means the authenticated owner no longer exists.
	ErrOwnerNotFound = errors.New("authenticated user no longer exists")
)

// ValidationError reports input that violates an invariant of a list or item.
type ValidationError struct {
	msg string
}

func (e *ValidationError) Error() string { return e.msg }

var (
	ErrEmptyUpdate        = &ValidationError{"update structure has no values"}
	ErrTitleRequired      = &ValidationError{"title is required"}
	ErrTitleTooLong       = &ValidationError{fmt.Sprintf("title must be at most %d characters", MaxTitleLength)}
	ErrDescriptionTooLong = &ValidationError{fmt.Sprintf("description must be at most %d characters", MaxDescriptionLength)}
	ErrNULCharacter       = &ValidationError{"text must not contain NUL characters"}
)
