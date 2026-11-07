package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/IlliaVern/go-todo-app/internal/platform/postgres/pgtest"
	"github.com/IlliaVern/go-todo-app/internal/todo/domain"
	"github.com/IlliaVern/go-todo-app/internal/todo/postgres"
)

func insertUser(t *testing.T, db *pgxpool.Pool, username string) domain.OwnerID {
	t.Helper()
	var id domain.OwnerID
	if err := db.QueryRow(t.Context(),
		`INSERT INTO users (name, username, password_hash) VALUES ('n', $1, 'h') RETURNING id`, username,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func newList(t *testing.T, owner domain.OwnerID, title string) domain.List {
	t.Helper()
	ti, d, err := domain.ParseText(title, "")
	if err != nil {
		t.Fatal(err)
	}
	return domain.NewList(owner, ti, d)
}

func newItem(t *testing.T, list domain.ListID, title string) domain.Item {
	t.Helper()
	ti, d, err := domain.ParseText(title, "")
	if err != nil {
		t.Fatal(err)
	}
	return domain.NewItem(list, ti, d)
}

// TestRepositoriesScopeEveryQueryToTheOwner calls every repository method as a
// different owner directly, bypassing the use-case checks, so a missing
// user_id filter in SQL fails here.
func TestRepositoriesScopeEveryQueryToTheOwner(t *testing.T) {
	db := pgtest.Start(t)
	ctx := context.Background()
	lists, items := postgres.NewListRepository(db), postgres.NewItemRepository(db)
	alice, bob := insertUser(t, db, "alice"), insertUser(t, db, "bob")

	listID, err := lists.Add(ctx, newList(t, alice, "Alice's"))
	if err != nil {
		t.Fatal(err)
	}
	item := newItem(t, listID, "secret")
	itemID, err := items.Add(ctx, alice, item)
	if err != nil {
		t.Fatal(err)
	}

	list, _ := lists.Get(ctx, alice, listID)
	stored, _ := items.Get(ctx, alice, itemID)
	hijackedList := domain.RestoreList(listID, bob, "hacked", "", list.Version())

	checks := []struct {
		name    string
		err     error
		wantErr error
	}{
		{"lists.Get", second(lists.Get(ctx, bob, listID)), domain.ErrListNotFound},
		{"lists.Update", second(lists.Update(ctx, hijackedList)), domain.ErrListNotFound},
		{"lists.Delete", lists.Delete(ctx, bob, listID), domain.ErrListNotFound},
		{"items.Add", second(items.Add(ctx, bob, item)), domain.ErrListNotFound},
		{"items.Get", second(items.Get(ctx, bob, itemID)), domain.ErrItemNotFound},
		{"items.Update", second(items.Update(ctx, bob, stored)), domain.ErrItemNotFound},
		{"items.Delete", items.Delete(ctx, bob, itemID), domain.ErrItemNotFound},
	}
	for _, c := range checks {
		if !errors.Is(c.err, c.wantErr) {
			t.Errorf("%s as another owner: err = %v, want %v", c.name, c.err, c.wantErr)
		}
	}

	if got, err := lists.ByOwner(ctx, bob); err != nil || len(got) != 0 {
		t.Errorf("lists.ByOwner(bob) = %v, %v; want empty", got, err)
	}
	if got, err := items.ByList(ctx, bob, listID); err != nil || len(got) != 0 {
		t.Errorf("items.ByList(bob) = %v, %v; want empty", got, err)
	}

	gotList, err := lists.Get(ctx, alice, listID)
	if err != nil || gotList != list {
		t.Errorf("alice's list changed: %+v, %v", gotList, err)
	}
	gotItem, err := items.Get(ctx, alice, itemID)
	if err != nil || gotItem != stored {
		t.Errorf("alice's item changed: %+v, %v", gotItem, err)
	}
}

func TestListDeletionCascadesToItems(t *testing.T) {
	db := pgtest.Start(t)
	ctx := context.Background()
	lists, items := postgres.NewListRepository(db), postgres.NewItemRepository(db)
	owner := insertUser(t, db, "owner")

	listID, _ := lists.Add(ctx, newList(t, owner, "L"))
	if _, err := items.Add(ctx, owner, newItem(t, listID, "I")); err != nil {
		t.Fatal(err)
	}

	if err := lists.Delete(ctx, owner, listID); err != nil {
		t.Fatal(err)
	}
	var remaining int
	if err := db.QueryRow(ctx, `SELECT count(*) FROM todo_items WHERE list_id = $1`, listID).Scan(&remaining); err != nil {
		t.Fatal(err)
	}
	if remaining != 0 {
		t.Errorf("%d items left after deleting their list", remaining)
	}
}

func TestAddListForMissingOwner(t *testing.T) {
	db := pgtest.Start(t)
	if _, err := postgres.NewListRepository(db).Add(context.Background(), newList(t, 987654321, "L")); !errors.Is(err, domain.ErrOwnerNotFound) {
		t.Errorf("err = %v, want %v", err, domain.ErrOwnerNotFound)
	}
}

func TestUpdateRejectsStaleVersions(t *testing.T) {
	db := pgtest.Start(t)
	ctx := context.Background()
	lists, items := postgres.NewListRepository(db), postgres.NewItemRepository(db)
	owner := insertUser(t, db, "owner")

	listID, _ := lists.Add(ctx, newList(t, owner, "L"))
	itemID, _ := items.Add(ctx, owner, newItem(t, listID, "I"))

	// Two writers load the same version; the first save wins.
	listA, _ := lists.Get(ctx, owner, listID)
	listB, _ := lists.Get(ctx, owner, listID)
	if err := listA.Apply(domain.ListPatch{Title: ptr("A")}); err != nil {
		t.Fatal(err)
	}
	savedA, err := lists.Update(ctx, listA)
	if err != nil {
		t.Fatal(err)
	}
	if savedA.Version() != listA.Version()+1 || savedA.Title().String() != "A" {
		t.Errorf("Update returned %+v, want version %d", savedA, listA.Version()+1)
	}
	if err := listB.Apply(domain.ListPatch{Description: ptr("B")}); err != nil {
		t.Fatal(err)
	}
	if _, err := lists.Update(ctx, listB); !errors.Is(err, domain.ErrConcurrentUpdate) {
		t.Errorf("stale list update: err = %v, want %v", err, domain.ErrConcurrentUpdate)
	}
	if got, _ := lists.Get(ctx, owner, listID); got != savedA {
		t.Errorf("list after race = %+v, want %+v", got, savedA)
	}

	itemA, _ := items.Get(ctx, owner, itemID)
	itemB, _ := items.Get(ctx, owner, itemID)
	_ = itemA.Apply(domain.ItemPatch{Done: ptr(true)})
	savedItem, err := items.Update(ctx, owner, itemA)
	if err != nil || savedItem.Version() != itemA.Version()+1 || !savedItem.Done() {
		t.Fatalf("item Update = %+v, %v", savedItem, err)
	}
	_ = itemB.Apply(domain.ItemPatch{Title: ptr("B")})
	if _, err := items.Update(ctx, owner, itemB); !errors.Is(err, domain.ErrConcurrentUpdate) {
		t.Errorf("stale item update: err = %v, want %v", err, domain.ErrConcurrentUpdate)
	}

	gone := domain.RestoreList(999999, owner, "x", "", 1)
	if _, err := lists.Update(ctx, gone); !errors.Is(err, domain.ErrListNotFound) {
		t.Errorf("missing list update: err = %v, want %v", err, domain.ErrListNotFound)
	}
}

func second[T any](_ T, err error) error { return err }

func ptr[T any](v T) *T { return &v }
