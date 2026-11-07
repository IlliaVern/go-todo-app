package postgres

import (
	"context"
	"errors"

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
	return domain.RestoreItem(r.ID, r.ListID, r.Title, r.Description, r.Done, r.Version)
}

type ItemRepository struct {
	db *pgxpool.Pool
}

func NewItemRepository(db *pgxpool.Pool) *ItemRepository {
	return &ItemRepository{db: db}
}

func (r *ItemRepository) Add(ctx context.Context, ownerID int64, item domain.Item) (int64, error) {
	var id int64
	// Inserting through a SELECT on the owner's list enforces ownership in SQL.
	err := r.db.QueryRow(ctx,
		`INSERT INTO todo_items (list_id, title, description, done)
		 SELECT tl.id, $2, $3, $4 FROM todo_lists tl WHERE tl.id = $1 AND tl.user_id = $5
		 RETURNING id`,
		item.ListID(), item.Title(), item.Description(), item.Done(), ownerID,
	).Scan(&id)
	if errors.Is(err, pgx.ErrNoRows) || postgres.IsForeignKeyViolation(err) {
		return 0, domain.ErrListNotFound
	}
	return id, err
}

func (r *ItemRepository) Get(ctx context.Context, ownerID, itemID int64) (domain.Item, error) {
	rows, _ := r.db.Query(ctx,
		`SELECT ti.id, ti.list_id, ti.title, ti.description, ti.done, ti.version
		   FROM todo_items ti
		   JOIN todo_lists tl ON tl.id = ti.list_id
		  WHERE ti.id = $1 AND tl.user_id = $2`,
		itemID, ownerID)
	row, err := pgx.CollectExactlyOneRow(rows, pgx.RowToStructByName[itemRow])
	if errors.Is(err, pgx.ErrNoRows) {
		return domain.Item{}, domain.ErrItemNotFound
	}
	if err != nil {
		return domain.Item{}, err
	}
	return row.toDomain(), nil
}

func (r *ItemRepository) ListByList(ctx context.Context, ownerID, listID int64) ([]domain.Item, error) {
	rows, _ := r.db.Query(ctx,
		`SELECT ti.id, ti.list_id, ti.title, ti.description, ti.done, ti.version
		   FROM todo_items ti
		   JOIN todo_lists tl ON tl.id = ti.list_id
		  WHERE ti.list_id = $1 AND tl.user_id = $2
		  ORDER BY ti.id`,
		listID, ownerID)
	itemRows, err := pgx.CollectRows(rows, pgx.RowToStructByName[itemRow])
	if err != nil {
		return nil, err
	}

	items := make([]domain.Item, 0, len(itemRows))
	for _, row := range itemRows {
		items = append(items, row.toDomain())
	}
	return items, nil
}

func (r *ItemRepository) Update(ctx context.Context, ownerID int64, item domain.Item) error {
	tag, err := r.db.Exec(ctx,
		`UPDATE todo_items ti
		    SET title = $1, description = $2, done = $3, version = ti.version + 1
		   FROM todo_lists tl
		  WHERE tl.id = ti.list_id AND ti.id = $4 AND tl.user_id = $5 AND ti.version = $6`,
		item.Title(), item.Description(), item.Done(), item.ID(), ownerID, item.Version())
	if err != nil || tag.RowsAffected() == 1 {
		return err
	}

	// No row matched: either the item is gone or it changed since it was loaded.
	if _, err := r.Get(ctx, ownerID, item.ID()); err != nil {
		return err
	}
	return domain.ErrConcurrentUpdate
}

func (r *ItemRepository) Delete(ctx context.Context, ownerID, itemID int64) error {
	tag, err := r.db.Exec(ctx,
		`DELETE FROM todo_items ti
		  USING todo_lists tl
		  WHERE tl.id = ti.list_id AND ti.id = $1 AND tl.user_id = $2`,
		itemID, ownerID)
	return affectedOne(tag, err, domain.ErrItemNotFound)
}
