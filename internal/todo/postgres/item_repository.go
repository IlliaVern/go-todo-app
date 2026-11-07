package postgres

import (
	"context"
	"database/sql"
	"errors"

	"github.com/jmoiron/sqlx"

	"github.com/IlliaVern/go-todo-app/internal/platform/postgres"
	"github.com/IlliaVern/go-todo-app/internal/todo/domain"
)

type itemRow struct {
	ID          int64  `db:"id"`
	ListID      int64  `db:"list_id"`
	Title       string `db:"title"`
	Description string `db:"description"`
	Done        bool   `db:"done"`
}

func (r itemRow) toDomain() domain.Item {
	return domain.RestoreItem(r.ID, r.ListID, r.Title, r.Description, r.Done)
}

type ItemRepository struct {
	db *sqlx.DB
}

func NewItemRepository(db *sqlx.DB) *ItemRepository {
	return &ItemRepository{db: db}
}

func (r *ItemRepository) Add(ctx context.Context, ownerID int64, item domain.Item) (int64, error) {
	var id int64
	// Inserting through a SELECT on the owner's list enforces ownership in SQL.
	err := r.db.QueryRowContext(ctx,
		`INSERT INTO todo_items (list_id, title, description, done)
		 SELECT tl.id, $2, $3, $4 FROM todo_lists tl WHERE tl.id = $1 AND tl.user_id = $5
		 RETURNING id`,
		item.ListID(), item.Title(), item.Description(), item.Done(), ownerID,
	).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) || postgres.IsForeignKeyViolation(err) {
		return 0, domain.ErrListNotFound
	}
	return id, err
}

func (r *ItemRepository) Get(ctx context.Context, ownerID, itemID int64) (domain.Item, error) {
	var row itemRow
	err := r.db.GetContext(ctx, &row,
		`SELECT ti.id, ti.list_id, ti.title, ti.description, ti.done
		   FROM todo_items ti
		   JOIN todo_lists tl ON tl.id = ti.list_id
		  WHERE ti.id = $1 AND tl.user_id = $2`,
		itemID, ownerID)
	if errors.Is(err, sql.ErrNoRows) {
		return domain.Item{}, domain.ErrItemNotFound
	}
	if err != nil {
		return domain.Item{}, err
	}
	return row.toDomain(), nil
}

func (r *ItemRepository) ListByList(ctx context.Context, ownerID, listID int64) ([]domain.Item, error) {
	var rows []itemRow
	if err := r.db.SelectContext(ctx, &rows,
		`SELECT ti.id, ti.list_id, ti.title, ti.description, ti.done
		   FROM todo_items ti
		   JOIN todo_lists tl ON tl.id = ti.list_id
		  WHERE ti.list_id = $1 AND tl.user_id = $2
		  ORDER BY ti.id`,
		listID, ownerID); err != nil {
		return nil, err
	}

	items := make([]domain.Item, 0, len(rows))
	for _, row := range rows {
		items = append(items, row.toDomain())
	}
	return items, nil
}

func (r *ItemRepository) Update(ctx context.Context, ownerID int64, item domain.Item) error {
	res, err := r.db.ExecContext(ctx,
		`UPDATE todo_items ti
		    SET title = $1, description = $2, done = $3
		   FROM todo_lists tl
		  WHERE tl.id = ti.list_id AND ti.id = $4 AND tl.user_id = $5`,
		item.Title(), item.Description(), item.Done(), item.ID(), ownerID)
	return affectedOne(res, err, domain.ErrItemNotFound)
}

func (r *ItemRepository) Delete(ctx context.Context, ownerID, itemID int64) error {
	res, err := r.db.ExecContext(ctx,
		`DELETE FROM todo_items ti
		  USING todo_lists tl
		  WHERE tl.id = ti.list_id AND ti.id = $1 AND tl.user_id = $2`,
		itemID, ownerID)
	return affectedOne(res, err, domain.ErrItemNotFound)
}
