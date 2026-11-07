package domain_test

import (
	"errors"
	"strings"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/todo/domain"
)

func ptr[T any](v T) *T { return &v }

func TestNewList(t *testing.T) {
	tests := []struct {
		name        string
		title       string
		description string
		wantErr     error
	}{
		{name: "valid", title: "Groceries", description: "weekly"},
		{name: "empty description", title: "Groceries"},
		{name: "empty title", title: "", wantErr: domain.ErrTitleRequired},
		{name: "blank title", title: "  \t", wantErr: domain.ErrTitleRequired},
		{name: "title at limit", title: strings.Repeat("я", 255)},
		{name: "title over limit", title: strings.Repeat("я", 256), wantErr: domain.ErrTitleTooLong},
		{name: "description over limit", title: "t", description: strings.Repeat("a", 256), wantErr: domain.ErrDescriptionTooLong},
		{name: "NUL in title", title: "a\x00b", wantErr: domain.ErrNULCharacter},
		{name: "NUL in description", title: "t", description: "a\x00b", wantErr: domain.ErrNULCharacter},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list, err := domain.NewList(7, tt.title, tt.description)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if err != nil {
				return
			}
			if list.OwnerID() != 7 || list.Title() != tt.title || list.Description() != tt.description || list.ID() != 0 {
				t.Errorf("got %+v", list)
			}
		})
	}
}

func TestListApply(t *testing.T) {
	tests := []struct {
		name            string
		patch           domain.ListPatch
		wantErr         error
		wantTitle       string
		wantDescription string
	}{
		{name: "empty patch", patch: domain.ListPatch{}, wantErr: domain.ErrEmptyUpdate, wantTitle: "old", wantDescription: "old desc"},
		{name: "title only", patch: domain.ListPatch{Title: ptr("new")}, wantTitle: "new", wantDescription: "old desc"},
		{name: "description only", patch: domain.ListPatch{Description: ptr("")}, wantTitle: "old", wantDescription: ""},
		{name: "both", patch: domain.ListPatch{Title: ptr("new"), Description: ptr("new desc")}, wantTitle: "new", wantDescription: "new desc"},
		{name: "blank title rejected", patch: domain.ListPatch{Title: ptr(" "), Description: ptr("new desc")}, wantErr: domain.ErrTitleRequired, wantTitle: "old", wantDescription: "old desc"},
		{name: "long description rejected", patch: domain.ListPatch{Description: ptr(strings.Repeat("a", 256))}, wantErr: domain.ErrDescriptionTooLong, wantTitle: "old", wantDescription: "old desc"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			list := domain.RestoreList(1, 7, "old", "old desc")
			err := list.Apply(tt.patch)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if list.Title() != tt.wantTitle || list.Description() != tt.wantDescription {
				t.Errorf("got title %q description %q, want %q %q", list.Title(), list.Description(), tt.wantTitle, tt.wantDescription)
			}
		})
	}
}

func TestValidationErrorsAreTyped(t *testing.T) {
	for _, err := range []error{domain.ErrEmptyUpdate, domain.ErrTitleRequired, domain.ErrTitleTooLong, domain.ErrDescriptionTooLong, domain.ErrNULCharacter} {
		var invalid *domain.ValidationError
		if !errors.As(err, &invalid) {
			t.Errorf("%v is not a *ValidationError", err)
		}
	}
	var invalid *domain.ValidationError
	if errors.As(domain.ErrListNotFound, &invalid) {
		t.Error("ErrListNotFound must not be a validation error")
	}
}
