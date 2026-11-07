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

func insertUser(t *testing.T, db *pgxpool.Pool, username string) int64 {
	t.Helper()
	var id int64
	if err := db.QueryRow(t.Context(),
		`INSERT INTO users (name, username, password_hash) VALUES ('n', $1, 'h') RETURNING id`, username,
	).Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

// TestRepositoriesScopeEveryQueryToTheOwner calls every repository method as a
// different owner directly, bypassing the use-case checks, so a missing
// user_id filter in SQL fails here.
func TestRepositoriesScopeEveryQueryToTheOwner(t *testing.T) {
	db := pgtest.Start(t)
	ctx := context.Background()
	lists, items := postgres.NewListRepository(db), postgres.NewItemRepository(db)
	alice, bob := insertUser(t, db, "alice"), insertUser(t, db, "bob")

	newList, _ := domain.NewList(alice, "Alice's", "")
	listID, err := lists.Add(ctx, newList)
	if err != nil {
		t.Fatal(err)
	}
	newItem, _ := domain.NewItem(listID, "secret", "")
	itemID, err := items.Add(ctx, alice, newItem)
	if err != nil {
		t.Fatal(err)
	}

	list, _ := lists.Get(ctx, alice, listID)
	item, _ := items.Get(ctx, alice, itemID)
	hijackedList := domain.RestoreList(listID, bob, "hacked", "", list.Version())

	checks := []struct {
		name    string
		err     error
		wantErr error
	}{
		{"lists.Get", second(lists.Get(ctx, bob, listID)), domain.ErrListNotFound},
		{"lists.Update", lists.Update(ctx, hijackedList), domain.ErrListNotFound},
		{"lists.Delete", lists.Delete(ctx, bob, listID), domain.ErrListNotFound},
		{"items.Add", second(items.Add(ctx, bob, newItem)), domain.ErrListNotFound},
		{"items.Get", second(items.Get(ctx, bob, itemID)), domain.ErrItemNotFound},
		{"items.Update", items.Update(ctx, bob, item), domain.ErrItemNotFound},
		{"items.Delete", items.Delete(ctx, bob, itemID), domain.ErrItemNotFound},
	}
	for _, c := range checks {
		if !errors.Is(c.err, c.wantErr) {
			t.Errorf("%s as another owner: err = %v, want %v", c.name, c.err, c.wantErr)
		}
	}

	if got, err := lists.ListByOwner(ctx, bob); err != nil || len(got) != 0 {
		t.Errorf("lists.ListByOwner(bob) = %v, %v; want empty", got, err)
	}
	if got, err := items.ListByList(ctx, bob, listID); err != nil || len(got) != 0 {
		t.Errorf("items.ListByList(bob) = %v, %v; want empty", got, err)
	}

	gotList, err := lists.Get(ctx, alice, listID)
	if err != nil || gotList != list {
		t.Errorf("alice's list changed: %+v, %v", gotList, err)
	}
	gotItem, err := items.Get(ctx, alice, itemID)
	if err != nil || gotItem != item {
		t.Errorf("alice's item changed: %+v, %v", gotItem, err)
	}
}

func TestListDeletionCascadesToItems(t *testing.T) {
	db := pgtest.Start(t)
	ctx := context.Background()
	lists, items := postgres.NewListRepository(db), postgres.NewItemRepository(db)
	owner := insertUser(t, db, "owner")

	newList, _ := domain.NewList(owner, "L", "")
	listID, _ := lists.Add(ctx, newList)
	newItem, _ := domain.NewItem(listID, "I", "")
	if _, err := items.Add(ctx, owner, newItem); err != nil {
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
	newList, _ := domain.NewList(987654321, "L", "")
	if _, err := postgres.NewListRepository(db).Add(context.Background(), newList); !errors.Is(err, domain.ErrOwnerNotFound) {
		t.Errorf("err = %v, want %v", err, domain.ErrOwnerNotFound)
	}
}

func second[T any](_ T, err error) error { return err }

func TestUpdateRejectsStaleVersions(t *testing.T) {
	db := pgtest.Start(t)
	ctx := context.Background()
	lists, items := postgres.NewListRepository(db), postgres.NewItemRepository(db)
	owner := insertUser(t, db, "owner")

	newList, _ := domain.NewList(owner, "L", "")
	listID, _ := lists.Add(ctx, newList)
	newItem, _ := domain.NewItem(listID, "I", "")
	itemID, _ := items.Add(ctx, owner, newItem)

	// Two writers load the same version; the first save wins.
	listA, _ := lists.Get(ctx, owner, listID)
	listB, _ := lists.Get(ctx, owner, listID)
	if err := listA.Apply(domain.ListPatch{Title: ptr("A")}); err != nil {
		t.Fatal(err)
	}
	if err := lists.Update(ctx, listA); err != nil {
		t.Fatal(err)
	}
	if err := listB.Apply(domain.ListPatch{Description: ptr("B")}); err != nil {
		t.Fatal(err)
	}
	if err := lists.Update(ctx, listB); !errors.Is(err, domain.ErrConcurrentUpdate) {
		t.Errorf("stale list update: err = %v, want %v", err, domain.ErrConcurrentUpdate)
	}
	if got, _ := lists.Get(ctx, owner, listID); got.Title() != "A" || got.Description() != "" || got.Version() != listA.Version()+1 {
		t.Errorf("list after race = %+v", got)
	}

	itemA, _ := items.Get(ctx, owner, itemID)
	itemB, _ := items.Get(ctx, owner, itemID)
	_ = itemA.Apply(domain.ItemPatch{Done: ptr(true)})
	if err := items.Update(ctx, owner, itemA); err != nil {
		t.Fatal(err)
	}
	_ = itemB.Apply(domain.ItemPatch{Title: ptr("B")})
	if err := items.Update(ctx, owner, itemB); !errors.Is(err, domain.ErrConcurrentUpdate) {
		t.Errorf("stale item update: err = %v, want %v", err, domain.ErrConcurrentUpdate)
	}

	gone := domain.RestoreList(999999, owner, "x", "", 1)
	if err := lists.Update(ctx, gone); !errors.Is(err, domain.ErrListNotFound) {
		t.Errorf("missing list update: err = %v, want %v", err, domain.ErrListNotFound)
	}
}

func ptr[T any](v T) *T { return &v }
