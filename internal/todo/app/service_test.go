package app_test

import (
	"context"
	"errors"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/todo/app"
	"github.com/IlliaVern/go-todo-app/internal/todo/domain"
)

// memStore is an in-memory implementation of both repository ports with the
// same owner scoping as the PostgreSQL adapters.
type memStore struct {
	lists  map[int64]domain.List
	items  map[int64]domain.Item
	nextID int64
}

func newMemStore() *memStore {
	return &memStore{lists: map[int64]domain.List{}, items: map[int64]domain.Item{}}
}

type memLists struct{ *memStore }
type memItems struct{ *memStore }

func (s memLists) Add(_ context.Context, l domain.List) (int64, error) {
	s.nextID++
	s.lists[s.nextID] = domain.RestoreList(s.nextID, l.OwnerID(), l.Title(), l.Description())
	return s.nextID, nil
}

func (s memLists) Get(_ context.Context, ownerID, listID int64) (domain.List, error) {
	l, ok := s.lists[listID]
	if !ok || l.OwnerID() != ownerID {
		return domain.List{}, domain.ErrListNotFound
	}
	return l, nil
}

func (s memLists) ListByOwner(_ context.Context, ownerID int64) ([]domain.List, error) {
	out := []domain.List{}
	for id := int64(1); id <= s.nextID; id++ {
		if l, ok := s.lists[id]; ok && l.OwnerID() == ownerID {
			out = append(out, l)
		}
	}
	return out, nil
}

func (s memLists) Update(ctx context.Context, l domain.List) error {
	if _, err := s.Get(ctx, l.OwnerID(), l.ID()); err != nil {
		return err
	}
	s.lists[l.ID()] = l
	return nil
}

func (s memLists) Delete(ctx context.Context, ownerID, listID int64) error {
	if _, err := s.Get(ctx, ownerID, listID); err != nil {
		return err
	}
	delete(s.lists, listID)
	for id, it := range s.items {
		if it.ListID() == listID {
			delete(s.items, id)
		}
	}
	return nil
}

func (s memItems) Add(_ context.Context, ownerID int64, it domain.Item) (int64, error) {
	if l, ok := s.lists[it.ListID()]; !ok || l.OwnerID() != ownerID {
		return 0, domain.ErrListNotFound
	}
	s.nextID++
	s.items[s.nextID] = domain.RestoreItem(s.nextID, it.ListID(), it.Title(), it.Description(), it.Done())
	return s.nextID, nil
}

func (s memItems) Get(_ context.Context, ownerID, itemID int64) (domain.Item, error) {
	it, ok := s.items[itemID]
	if !ok || s.lists[it.ListID()].OwnerID() != ownerID {
		return domain.Item{}, domain.ErrItemNotFound
	}
	return it, nil
}

func (s memItems) ListByList(_ context.Context, ownerID, listID int64) ([]domain.Item, error) {
	out := []domain.Item{}
	for id := int64(1); id <= s.nextID; id++ {
		if it, ok := s.items[id]; ok && it.ListID() == listID && s.lists[listID].OwnerID() == ownerID {
			out = append(out, it)
		}
	}
	return out, nil
}

func (s memItems) Update(ctx context.Context, ownerID int64, it domain.Item) error {
	if _, err := s.Get(ctx, ownerID, it.ID()); err != nil {
		return err
	}
	s.items[it.ID()] = it
	return nil
}

func (s memItems) Delete(ctx context.Context, ownerID, itemID int64) error {
	if _, err := s.Get(ctx, ownerID, itemID); err != nil {
		return err
	}
	delete(s.items, itemID)
	return nil
}

const (
	alice int64 = 1
	bob   int64 = 2
)

func newService() *app.Service {
	store := newMemStore()
	return app.NewService(memLists{store}, memItems{store})
}

func ptr[T any](v T) *T { return &v }

func TestListLifecycle(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	listID, err := svc.CreateList(ctx, alice, "Groceries", "")
	if err != nil {
		t.Fatal(err)
	}

	if err := svc.UpdateList(ctx, alice, listID, domain.ListPatch{Description: ptr("weekly")}); err != nil {
		t.Fatal(err)
	}
	list, err := svc.List(ctx, alice, listID)
	if err != nil || list.Title() != "Groceries" || list.Description() != "weekly" {
		t.Fatalf("List = %+v, %v", list, err)
	}

	itemID, err := svc.AddItem(ctx, alice, listID, "Milk", "")
	if err != nil {
		t.Fatal(err)
	}
	if err := svc.UpdateItem(ctx, alice, itemID, domain.ItemPatch{Done: ptr(true)}); err != nil {
		t.Fatal(err)
	}
	items, err := svc.Items(ctx, alice, listID)
	if err != nil || len(items) != 1 || !items[0].Done() || items[0].Title() != "Milk" {
		t.Fatalf("Items = %+v, %v", items, err)
	}

	if err := svc.DeleteList(ctx, alice, listID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.Item(ctx, alice, itemID); !errors.Is(err, domain.ErrItemNotFound) {
		t.Errorf("item after list deletion: err = %v, want %v", err, domain.ErrItemNotFound)
	}
}

func TestOtherOwnersDataIsInvisible(t *testing.T) {
	ctx := context.Background()
	svc := newService()
	listID, _ := svc.CreateList(ctx, alice, "Alice's", "")
	itemID, _ := svc.AddItem(ctx, alice, listID, "secret", "")

	checks := []struct {
		name    string
		err     error
		wantErr error
	}{
		{"get list", second(svc.List(ctx, bob, listID)), domain.ErrListNotFound},
		{"update list", svc.UpdateList(ctx, bob, listID, domain.ListPatch{Title: ptr("x")}), domain.ErrListNotFound},
		{"delete list", svc.DeleteList(ctx, bob, listID), domain.ErrListNotFound},
		{"add item", second(svc.AddItem(ctx, bob, listID, "x", "")), domain.ErrListNotFound},
		{"list items", second(svc.Items(ctx, bob, listID)), domain.ErrListNotFound},
		{"get item", second(svc.Item(ctx, bob, itemID)), domain.ErrItemNotFound},
		{"update item", svc.UpdateItem(ctx, bob, itemID, domain.ItemPatch{Done: ptr(true)}), domain.ErrItemNotFound},
		{"delete item", svc.DeleteItem(ctx, bob, itemID), domain.ErrItemNotFound},
	}
	for _, c := range checks {
		if !errors.Is(c.err, c.wantErr) {
			t.Errorf("%s: err = %v, want %v", c.name, c.err, c.wantErr)
		}
	}

	lists, _ := svc.Lists(ctx, bob)
	if len(lists) != 0 {
		t.Errorf("bob sees %d lists, want 0", len(lists))
	}
	item, err := svc.Item(ctx, alice, itemID)
	if err != nil || item.Title() != "secret" || item.Done() {
		t.Errorf("alice's item changed: %+v, %v", item, err)
	}
}

func TestValidationHappensBeforePersistence(t *testing.T) {
	ctx := context.Background()
	svc := newService()

	if _, err := svc.CreateList(ctx, alice, "", ""); !errors.Is(err, domain.ErrTitleRequired) {
		t.Errorf("CreateList: err = %v, want %v", err, domain.ErrTitleRequired)
	}
	if lists, _ := svc.Lists(ctx, alice); len(lists) != 0 {
		t.Errorf("invalid list was stored")
	}

	listID, _ := svc.CreateList(ctx, alice, "L", "")
	if err := svc.UpdateList(ctx, alice, listID, domain.ListPatch{}); !errors.Is(err, domain.ErrEmptyUpdate) {
		t.Errorf("UpdateList: err = %v, want %v", err, domain.ErrEmptyUpdate)
	}
}

func second[T any](_ T, err error) error { return err }
