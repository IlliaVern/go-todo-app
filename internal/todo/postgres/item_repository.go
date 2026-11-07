package postgres

import (
	"context"
	"errors"
	"fmt"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/IlliaVern/go-todo-app/internal/platform/postgres"
	"github.com/IlliaVern/go-todo-app/internal/todo/domain"
)

type itemRow struct {
	ID          int64  `db:"id"`
	ListID      int64  `db:"list_id"`
	Title       string `db:"title"`
	Description string `db:"description"`
	Done        bool   `db:"done"`
	Version     int64  `db:"version"`
}

func (r itemRow) toDomain() domain.Item {
	return domain.RestoreItem(domain.ItemID(r.ID), domain.ListID(r.ListID), r.Title, r.Description, r.Done, r.Version)
}

const itemColumns = `ti.id, ti.list_id, ti.title, ti.description, ti.done, ti.version`

type ItemRepository struct {
	db *pgxpool.Pool
}

func NewItemRepository(db *pgxpool.Pool) *ItemRepository {
	return &ItemRepository{db: db}
}

func (r *ItemRepository) Add(ctx context.Context, owner domain.OwnerID, item domain.Item) (domain.ItemID, error) {
	var id domain.ItemID
	// Inserting through a SELECT on the owner's list enforces ownership in SQL.
	err := r.db.QueryRow(ctx,
		`INSERT INTO todo_items (list_id, title, description, done)
		 SELECT tl.id, $2, $3, $4 FROM todo_lists tl WHERE tl.id = $1 AND tl.user_id = $5
		 RETURNING id`,
		item.ListID(), item.Title().String(), item.Description().String(), item.Done(), owner,
	).Scan(&id)
	switch {
	case errors.Is(err, pgx.ErrNoRows), postgres.IsForeignKeyViolation(err):
		return 0, domain.ErrListNotFound
	case err != nil:
		return 0, fmt.Errorf("insert todo item: %w", err)
	}
	return id, nil
}

func (r *ItemRepository) Get(ctx context.Context, owner domain.OwnerID, id domain.ItemID) (domain.Item, error) {
	rows, _ := r.db.Query(ctx,
		`SELECT `+itemColumns+`
		   FROM todo_items ti
		   JOIN todo_lists tl ON tl.id = ti.list_id
		  WHERE ti.id = $1 AND tl.user_id = $2`,
		id, owner)
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[itemRow])
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.Item{}, domain.ErrItemNotFound
	case err != nil:
		return domain.Item{}, fmt.Errorf("select todo item: %w", err)
	}
	return row.toDomain(), nil
}

func (r *ItemRepository) ByList(ctx context.Context, owner domain.OwnerID, list domain.ListID) ([]domain.Item, error) {
	rows, _ := r.db.Query(ctx,
		`SELECT `+itemColumns+`
		   FROM todo_items ti
		   JOIN todo_lists tl ON tl.id = ti.list_id
		  WHERE ti.list_id = $1 AND tl.user_id = $2
		  ORDER BY ti.id`,
		list, owner)
	itemRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[itemRow])
	if err != nil {
		return nil, fmt.Errorf("select todo items: %w", err)
	}

	items := make([]domain.Item, 0, len(itemRows))
	for _, row := range itemRows {
		items = append(items, row.toDomain())
	}
	return items, nil
}

func (r *ItemRepository) Update(ctx context.Context, owner domain.OwnerID, item domain.Item) (domain.Item, error) {
	rows, _ := r.db.Query(ctx,
		`UPDATE todo_items ti
		    SET title = $1, description = $2, done = $3, version = ti.version + 1
		   FROM todo_lists tl
		  WHERE tl.id = ti.list_id AND ti.id = $4 AND tl.user_id = $5 AND ti.version = $6
		  RETURNING `+itemColumns,
		item.Title().String(), item.Description().String(), item.Done(), item.ID(), owner, item.Version())
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[itemRow])
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return domain.Item{}, missingOrConflict(r.Get(ctx, owner, item.ID()))
	case err != nil:
		return domain.Item{}, fmt.Errorf("update todo item: %w", err)
	}
	return row.toDomain(), nil
}

func (r *ItemRepository) Delete(ctx context.Context, owner domain.OwnerID, id domain.ItemID) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM todo_items ti
		  USING todo_lists tl
		  WHERE tl.id = ti.list_id AND ti.id = $1 AND tl.user_id = $2`,
		id, owner)
	if err != nil {
		return fmt.Errorf("delete todo item: %w", err)
	}
	return affectedOne(tag, domain.ErrItemNotFound)
}
