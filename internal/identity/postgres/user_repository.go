// Package postgres implements the identity repositories on PostgreSQL.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
	"github.com/IlliaVern/go-todo-app/internal/platform/postgres"
)

const userColumns = `id, name, username, password_hash, version`

type userRow struct {
	ID           int64  `db:"id"`
	Name         string `db:"name"`
	Username     string `db:"username"`
	PasswordHash string `db:"password_hash"`
	Version      int64  `db:"version"`
}

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Add(ctx context.Context, user domain.User) (domain.UserID, error) {
	var id int64
	err := r.db.QueryRow(ctx,
		`INSERT INTO users (name, username, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		user.Name().String(), user.Username().String(), string(user.PasswordHash()),
	).Scan(&id)
	if postgres.IsUniqueViolation(err) {
		return 0, domain.ErrUsernameTaken
	}
	if err != nil {
		return 0, fmt.Errorf("insert user: %w", err)
	}
	return domain.UserID(id), nil
}

func (r *UserRepository) Get(ctx context.Context, id domain.UserID) (domain.User, error) {
	rows, _ := r.db.Query(ctx, `SELECT `+userColumns+` FROM users WHERE id = $1`, id)
	return collectUser(rows)
}

func (r *UserRepository) ByUsername(ctx context.Context, username domain.Username) (domain.User, error) {
	rows, _ := r.db.Query(ctx, `SELECT `+userColumns+` FROM users WHERE username = $1`, username.String())
	return collectUser(rows)
}

func collectUser(rows pgx.Rows) (domain.User, error) {
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[userRow])
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, fmt.Errorf("select user: %w", err)
	}
	return domain.RestoreUser(domain.UserID(row.ID), row.Name, row.Username, domain.PasswordHash(row.PasswordHash), row.Version), nil
}

func (r *UserRepository) Exists(ctx context.Context, id domain.UserID) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, id).Scan(&exists)
	if err != nil {
		return false, fmt.Errorf("check user exists: %w", err)
	}
	return exists, nil
}

func (r *UserRepository) Update(ctx context.Context, user domain.User) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE users SET name = $1, username = $2, password_hash = $3, version = version + 1
		  WHERE id = $4 AND version = $5`,
		user.Name().String(), user.Username().String(), string(user.PasswordHash()), user.ID(), user.Version())
	if postgres.IsUniqueViolation(err) {
		return domain.ErrUsernameTaken
	}
	if err != nil {
		return fmt.Errorf("update user: %w", err)
	}
	if tag.RowsAffected() > 0 {
		return nil
	}
	// No row matched: either the user is gone or another writer changed it first.
	if _, err := r.Get(ctx, user.ID()); err != nil {
		return err
	}
	return domain.ErrConcurrentUpdate
}

func (r *UserRepository) Delete(ctx context.Context, id domain.UserID) error {
	tag, err := r.db.Exec(ctx, `DELETE FROM users WHERE id = $1`, id)
	if err != nil {
		return fmt.Errorf("delete user: %w", err)
	}
	if tag.RowsAffected() == 0 {
		return domain.ErrUserNotFound
	}
	return nil
}
