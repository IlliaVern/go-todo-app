// Package postgres implements the todo repositories on PostgreSQL. Ownership
// is enforced in SQL: lists filter on user_id, items join their list.
package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/IlliaVern/go-todo-app/internal/platform/postgres"
	"github.com/IlliaVern/go-todo-app/internal/todo/domain"
)

type listRow struct {
	ID          int64  `db:"id"`
	UserID      int64  `db:"user_id"`
	Title       string `db:"title"`
	Description string `db:"description"`
	Version     int64  `db:"version"`
}

func (r listRow) toDomain() domain.List {
	return domain.RestoreList(domain.ListID(r.ID), domain.OwnerID(r.UserID), r.Title, r.Description, r.Version)
}

const listColumns = `id, user_id, title, description, version`

type ListRepository struct {
	db *pgxpool.Pool
}

func NewListRepository(db *pgxpool.Pool) *ListRepository {
	return &ListRepository{db: db}
}

func (r *ListRepository) Add(ctx context.Context, list domain.List) (domain.ListID, error) {
	var id domain.ListID
	err := r.db.QueryRow(ctx,
		`INSERT INTO todo_lists (user_id, title, description) VALUES ($1, $2, $3) RETURNING id`,
		list.OwnerID(), list.Title().String(), list.Description().String(),
	).Scan(&id)
	switch {
	case postgres.IsForeignKeyViolation(err):
		return 0, domain.ErrOwnerNotFound
	case err != nil:
		return 0, fmt.Errorf("insert todo list: %w", err)
	}
	return id, nil
}

func (r *ListRepository) Get(ctx context.Context, owner domain.OwnerID, id domain.ListID) (domain.List, error) {
	rows, _ := r.db.Query(ctx,
		`SELECT `+listColumns+` FROM todo_lists WHERE id = $1 AND user_id = $2`,
		id, owner)
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[listRow])
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.List{}, domain.ErrListNotFound
	case err != nil:
		return domain.List{}, fmt.Errorf("select todo list: %w", err)
	}
	return row.toDomain(), nil
}

func (r *ListRepository) ByOwner(ctx context.Context, owner domain.OwnerID) ([]domain.List, error) {
	rows, _ := r.db.Query(ctx,
		`SELECT `+listColumns+` FROM todo_lists WHERE user_id = $1 ORDER BY id`,
		owner)
	listRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[listRow])
	if err != nil {
		return nil, fmt.Errorf("select todo lists: %w", err)
	}

	lists := make([]domain.List, 0, len(listRows))
	for _, row := range listRows {
		lists = append(lists, row.toDomain())
	}
	return lists, nil
}

func (r *ListRepository) Update(ctx context.Context, list domain.List) (domain.List, error) {
	rows, _ := r.db.Query(ctx,
		`UPDATE todo_lists SET title = $1, description = $2, version = version + 1
		  WHERE id = $3 AND user_id = $4 AND version = $5
		  RETURNING `+listColumns,
		list.Title().String(), list.Description().String(), list.ID(), list.OwnerID(), list.Version())
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[listRow])
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.List{}, missingOrConflict(r.Get(ctx, list.OwnerID(), list.ID()))
	case err != nil:
		return domain.List{}, fmt.Errorf("update todo list: %w", err)
	}
	return row.toDomain(), nil
}

func (r *ListRepository) Delete(ctx context.Context, owner domain.OwnerID, id domain.ListID) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM todo_lists WHERE id = $1 AND user_id = $2`,
		id, owner)
	if err != nil {
		return fmt.Errorf("delete todo list: %w", err)
	}
	return affectedOne(tag, domain.ErrListNotFound)
}

// missingOrConflict explains a version-checked update that matched no row,
// given the owner-scoped reload of the same aggregate: if it still exists,
// another writer changed it first.
func missingOrConflict[T any](_ T, reloadErr error) error {
	if reloadErr != nil {
		return reloadErr
	}
	return domain.ErrConcurrentUpdate
}

func affectedOne(tag pgconn.CommandTag, notFound error) error {
	if tag.RowsAffected() == 0 {
		return notFound
	}
	return nil
}
