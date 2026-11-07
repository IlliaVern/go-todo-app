// Package postgres implements the identity repositories on PostgreSQL.
package postgres

import (
	"context"
	"errors"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/IlliaVern/go-todo-app/internal/identity/domain"
	"github.com/IlliaVern/go-todo-app/internal/platform/postgres"
)

type userRow struct {
	ID           int64  `db:"id"`
	Name         string `db:"name"`
	Username     string `db:"username"`
	PasswordHash string `db:"password_hash"`
}

type UserRepository struct {
	db *pgxpool.Pool
}

func NewUserRepository(db *pgxpool.Pool) *UserRepository {
	return &UserRepository{db: db}
}

func (r *UserRepository) Add(ctx context.Context, user domain.User) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx,
		`INSERT INTO users (name, username, password_hash) VALUES ($1, $2, $3) RETURNING id`,
		user.Name().String(), user.Username().String(), user.PasswordHash(),
	).Scan(&id)
	if postgres.IsUniqueViolation(err) {
		return 0, domain.ErrUsernameTaken
	}
	return id, err
}

func (r *UserRepository) ByUsername(ctx context.Context, username domain.Username) (domain.User, error) {
	rows, _ := r.db.Query(ctx,
		`SELECT id, name, username, password_hash FROM users WHERE username = $1`,
		username.String())
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[userRow])
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.User{}, domain.ErrUserNotFound
	}
	if err != nil {
		return domain.User{}, err
	}
	return domain.RestoreUser(row.ID, row.Name, row.Username, row.PasswordHash), nil
}

func (r *UserRepository) Exists(ctx context.Context, id int64) (bool, error) {
	var exists bool
	err := r.db.QueryRow(ctx, `SELECT EXISTS (SELECT 1 FROM users WHERE id = $1)`, id).Scan(&exists)
	return exists, err
}
