// Package postgres implements the todo repositories on PostgreSQL. Ownership
// is enforced in SQL: lists filter on user_id, items join their list.
package postgres

import (
	"context"
	"errors"

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
}

func (r listRow) toDomain() domain.List {
	return domain.RestoreList(r.ID, r.UserID, r.Title, r.Description)
}

type ListRepository struct {
	db *pgxpool.Pool
}

func NewListRepository(db *pgxpool.Pool) *ListRepository {
	return &ListRepository{db: db}
}

func (r *ListRepository) Add(ctx context.Context, list domain.List) (int64, error) {
	var id int64
	err := r.db.QueryRow(ctx,
		`INSERT INTO todo_lists (user_id, title, description) VALUES ($1, $2, $3) RETURNING id`,
		list.OwnerID(), list.Title(), list.Description(),
	).Scan(&id)
	if postgres.IsForeignKeyViolation(err) {
		return 0, domain.ErrOwnerNotFound
	}
	return id, err
}

func (r *ListRepository) Get(ctx context.Context, ownerID, listID int64) (domain.List, error) {
	rows, _ := r.db.Query(ctx,
		`SELECT id, user_id, title, description FROM todo_lists WHERE id = $1 AND user_id = $2`,
		listID, ownerID)
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[listRow])
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.List{}, domain.ErrListNotFound
	}
	if err != nil {
		return domain.List{}, err
	}
	return row.toDomain(), nil
}

func (r *ListRepository) ListByOwner(ctx context.Context, ownerID int64) ([]domain.List, error) {
	rows, _ := r.db.Query(ctx,
		`SELECT id, user_id, title, description FROM todo_lists WHERE user_id = $1 ORDER BY id`,
		ownerID)
	listRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[listRow])
	if err != nil {
		return nil, err
	}

	lists := make([]domain.List, 0, len(listRows))
	for _, row := range listRows {
		lists = append(lists, row.toDomain())
	}
	return lists, nil
}

func (r *ListRepository) Update(ctx context.Context, list domain.List) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE todo_lists SET title = $1, description = $2 WHERE id = $3 AND user_id = $4`,
		list.Title(), list.Description(), list.ID(), list.OwnerID())
	return affectedOne(tag, err, domain.ErrListNotFound)
}

func (r *ListRepository) Delete(ctx context.Context, ownerID, listID int64) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM todo_lists WHERE id = $1 AND user_id = $2`,
		listID, ownerID)
	return affectedOne(tag, err, domain.ErrListNotFound)
}

// affectedOne turns "no row matched" into notFound.
func affectedOne(tag pgconn.CommandTag, err, notFound error) error {
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return notFound
	}
	return nil
}
