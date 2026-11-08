package postgres_test

import (
	"context"
	"errors"
	"testing"

	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
	"github.com/IlliaVern/go-todo-app/internal/identity/postgres"
	"github.com/IlliaVern/go-todo-app/internal/platform/postgres/pgtest"
)

func newUser(t *testing.T, username string) domain.User {
	t.Helper()
	name, err := domain.NewName("Ada Lovelace")
	if err != nil {
		t.Fatal(err)
	}
	un, err := domain.NewUsername(username)
	if err != nil {
		t.Fatal(err)
	}
	user, err := domain.NewUser(name, un, "hash")
	if err != nil {
		t.Fatal(err)
	}
	return user
}

func ptr[T any](v T) *T { return &v }

func TestUserRepository(t *testing.T) {
	repo := postgres.NewUserRepository(pgtest.Start(t))
	ctx := context.Background()

	user := newUser(t, "Ada")

	id, err := repo.Add(ctx, user)
	if err != nil || id <= 0 {
		t.Fatalf("Add = %d, %v", id, err)
	}
	if _, err := repo.Add(ctx, user); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Errorf("duplicate Add: err = %v, want %v", err, domain.ErrUsernameTaken)
	}

	got, err := repo.ByUsername(ctx, user.Username())
	if err != nil {
		t.Fatal(err)
	}
	if got.ID() != id || got.Name() != user.Name() || got.Username() != user.Username() || got.PasswordHash() != "hash" || got.Version() != 1 {
		t.Errorf("ByUsername = %+v", got)
	}

	unknown, _ := domain.NewUsername("nobody")
	if _, err := repo.ByUsername(ctx, unknown); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("unknown username: err = %v, want %v", err, domain.ErrUserNotFound)
	}

	for _, tt := range []struct {
		id   domain.UserID
		want bool
	}{{id, true}, {id + 1000, false}} {
		if exists, err := repo.Exists(ctx, tt.id); err != nil || exists != tt.want {
			t.Errorf("Exists(%d) = %v, %v; want %v", tt.id, exists, err, tt.want)
		}
	}
}

func TestDeleteRemovesTheUsersListsAndItems(t *testing.T) {
	db := pgtest.Start(t)
	repo := postgres.NewUserRepository(db)
	ctx := context.Background()

	id, err := repo.Add(ctx, newUser(t, "ada"))
	if err != nil {
		t.Fatal(err)
	}
	var listID int64
	if err := db.QueryRow(ctx, `INSERT INTO todo_lists (user_id, title) VALUES ($1, 'L') RETURNING id`, id).Scan(&listID); err != nil {
		t.Fatal(err)
	}
	if _, err := db.Exec(ctx, `INSERT INTO todo_items (list_id, title) VALUES ($1, 'I')`, listID); err != nil {
		t.Fatal(err)
	}

	if err := repo.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}

	for _, table := range []string{"todo_lists", "todo_items"} {
		var remaining int
		if err := db.QueryRow(ctx, `SELECT count(*) FROM `+table).Scan(&remaining); err != nil {
			t.Fatal(err)
		}
		if remaining != 0 {
			t.Errorf("%s: %d rows left after deleting the user", table, remaining)
		}
	}
}

func TestUserRepositoryUpdateAndDelete(t *testing.T) {
	repo := postgres.NewUserRepository(pgtest.Start(t))
	ctx := context.Background()

	id, err := repo.Add(ctx, newUser(t, "ada"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Add(ctx, newUser(t, "bob")); err != nil {
		t.Fatal(err)
	}

	loaded, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	stale := loaded

	if err := loaded.Apply(domain.ProfilePatch{Name: ptr("Ada L."), Username: ptr("lovelace")}); err != nil {
		t.Fatal(err)
	}
	if err := loaded.SetPasswordHash("new-hash"); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, loaded); err != nil {
		t.Fatal(err)
	}
	saved, err := repo.Get(ctx, id)
	if err != nil {
		t.Fatal(err)
	}
	if saved.Name().String() != "Ada L." || saved.Username().String() != "lovelace" || saved.PasswordHash() != "new-hash" || saved.Version() != 2 {
		t.Errorf("after Update: %+v", saved)
	}

	if err := repo.Update(ctx, stale); !errors.Is(err, domain.ErrConcurrentUpdate) {
		t.Errorf("stale version: err = %v, want %v", err, domain.ErrConcurrentUpdate)
	}

	if err := saved.Apply(domain.ProfilePatch{Username: ptr("bob")}); err != nil {
		t.Fatal(err)
	}
	if err := repo.Update(ctx, saved); !errors.Is(err, domain.ErrUsernameTaken) {
		t.Errorf("taken username: err = %v, want %v", err, domain.ErrUsernameTaken)
	}

	if err := repo.Delete(ctx, id); err != nil {
		t.Fatal(err)
	}
	if _, err := repo.Get(ctx, id); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("Get after Delete: err = %v, want %v", err, domain.ErrUserNotFound)
	}
	if err := repo.Delete(ctx, id); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("second Delete: err = %v, want %v", err, domain.ErrUserNotFound)
	}
	if err := repo.Update(ctx, stale); !errors.Is(err, domain.ErrUserNotFound) {
		t.Errorf("Update after Delete: err = %v, want %v", err, domain.ErrUserNotFound)
	}
}
