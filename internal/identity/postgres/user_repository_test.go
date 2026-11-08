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
	if got.ID() != id || got.Name() != user.Name() || got.Username() != user.Username() || got.PasswordHash() != "hash" {
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
