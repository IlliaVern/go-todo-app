package repository

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"github.com/IlliaVern/go-todo-app"
	"github.com/jmoiron/sqlx"
)

type AuthPostgres struct {
	db *sqlx.DB
}

func NewAuthPostgres(db *sqlx.DB) *AuthPostgres {
	return &AuthPostgres{db: db}
}

// CreateUser stores the user; user.Password must already contain the password hash.
func (r *AuthPostgres) CreateUser(ctx context.Context, user todo.User) (int, error) {
	var id int
	query := fmt.Sprintf("INSERT INTO %s (name, username, password_hash) values ($1, $2, $3) RETURNING id", usersTable)

	row := r.db.QueryRowContext(ctx, query, user.Name, user.Username, user.Password)
	if err := row.Scan(&id); err != nil {
		return 0, err
	}

	return id, nil
}

// GetUser returns the user with the given username; Password holds the stored password hash.
func (r *AuthPostgres) GetUser(ctx context.Context, username string) (todo.User, error) {
	var user todo.User
	query := fmt.Sprintf("SELECT id, name, username, password_hash FROM %s WHERE username = $1", usersTable)

	row := r.db.QueryRowContext(ctx, query, username)
	if err := row.Scan(&user.Id, &user.Name, &user.Username, &user.Password); err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return todo.User{}, todo.ErrNotFound
		}
		return todo.User{}, err
	}

	return user, nil
}
