// Package postgres implements the todo repositories on PostgreSQL. Ownership
// is enforced in SQL: lists filter on user_id, items join their list.
package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"

	"github.com/IlliaVern/go-todo-app/internal/platform/postgres"
	"github.com/IlliaVern/go-todo-app/internal/todo/domain"
)

type listRow struct {
	ID          int64  `db:"id"`
	UserID      int64  `db:"user_id"`
	Title       string `db:"title"`
	Description string `db:"description"`
}

func (r listRow) toDomain() domain.List {
	return domain.RestoreList(r.ID, r.UserID, r.Title, r.Description)
}

type ListRepository struct {
	db *sqlx.DB
}

func NewListRepository(db *sqlx.DB) *ListRepository {
	return &ListRepository{db: db}
}

func (r *ListRepository) Add(ctx context.Context, list domain.List) (int64, error) {
	var id int64
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO todo_lists (user_id, title, description) VALUES ($1, $2, $3) RETURNING id`,
		list.OwnerID(), list.Title(), list.Description(),
	).Scan(&id)
	if postgres.IsForeignKeyViolation(err) {
		return 0, domain.ErrOwnerNotFound
	}
	return id, err
}

func (r *ListRepository) Get(ctx context.Context, ownerID, listID int64) (domain.List, error) {
	var row listRow
	err := r.db.GetContext(ctx, &row,
		`SELECT id, user_id, title, description FROM todo_lists WHERE id = $1 AND user_id = $2`,
		listID, ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.List{}, domain.ErrListNotFound
	}
	if err != nil {
		return domain.List{}, err
	}
	return row.toDomain(), nil
}

func (r *ListRepository) ListByOwner(ctx context.Context, ownerID int64) ([]domain.List, error) {
	var rows []listRow
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT id, user_id, title, description FROM todo_lists WHERE user_id = $1 ORDER BY id`,
		ownerID); err != nil {
		return nil, err
	}

	lists := make([]domain.List, 0, len(rows))
	for _, row := range rows {
		lists = append(lists, row.toDomain())
	}
	return lists, nil
}

func (r *ListRepository) Update(ctx context.Context, list domain.List) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE todo_lists SET title = $1, description = $2 WHERE id = $3 AND user_id = $4`,
		list.Title(), list.Description(), list.ID(), list.OwnerID())
	return affectedOne(res, err, domain.ErrListNotFound)
}

func (r *ListRepository) Delete(ctx context.Context, ownerID, listID int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM todo_lists WHERE id = $1 AND user_id = $2`,
		listID, ownerID)
	return affectedOne(res, err, domain.ErrListNotFound)
}

// affectedOne turns "no row matched" into notFound.
func affectedOne(res sql.Result, err error, notFound error) error {
	if err != nil {
		return err
	}
	n, err := res.RowsAffected()
	if err != nil {
		return err
	}
	if n == 0 {
		return notFound
	}
	return nil
}
