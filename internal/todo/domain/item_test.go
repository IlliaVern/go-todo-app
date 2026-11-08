package domain_test

import (
	"errors"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/todo/domain"
)

func TestNewItem(t *testing.T) {
	title, description, _ := domain.ParseText(domain.TextInput{Title: "Milk", Description: "2 l"})
	item := domain.NewItem(3, title, description)
	if item.ListID() != 3 || item.Title() != title || item.Description() != description || item.Done() || item.ID() != 0 {
		t.Errorf("got %+v", item)
	}
}

func TestItemApply(t *testing.T) {
	tests := []struct {
		name      string
		patch     domain.ItemPatch
		wantErr   error
		wantTitle string
		wantDone  bool
	}{
		{name: "empty patch", patch: domain.ItemPatch{}, wantErr: domain.ErrEmptyUpdate, wantTitle: "old"},
		{name: "done only", patch: domain.ItemPatch{Done: ptr(true)}, wantTitle: "old", wantDone: true},
		{name: "done false is a change", patch: domain.ItemPatch{Done: ptr(false)}, wantTitle: "old"},
		{name: "title and done", patch: domain.ItemPatch{Title: ptr("new"), Done: ptr(true)}, wantTitle: "new", wantDone: true},
		{name: "invalid title keeps done", patch: domain.ItemPatch{Title: ptr(""), Done: ptr(true)}, wantErr: domain.ErrTitleRequired, wantTitle: "old"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			item := domain.RestoreItem(1, 3, "old", "desc", false, 1)
			err := item.Apply(tt.patch)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("err = %v, want %v", err, tt.wantErr)
			}
			if item.Title().String() != tt.wantTitle || item.Done() != tt.wantDone || item.Description().String() != "desc" {
				t.Errorf("got %+v", item)
			}
		})
	}
}
