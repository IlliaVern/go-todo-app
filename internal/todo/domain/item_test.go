package domain_test

import (
	"errors"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/todo/domain"
)

func TestNewItem(t *testing.T) {
	item, err := domain.NewItem(3, "Milk", "2 l")
	if err != nil {
		t.Fatal(err)
	}
	if item.ListID() != 3 || item.Title().String() != "Milk" || item.Description().String() != "2 l" || item.Done() {
		t.Errorf("got %+v", item)
	}

	if _, err := domain.NewItem(3, "", ""); !errors.Is(err, domain.ErrTitleRequired) {
		t.Errorf("empty title: err = %v, want %v", err, domain.ErrTitleRequired)
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
