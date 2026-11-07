package repository

import (
	"context"
	"fmt"
	"log/slog"
	"strings"

	"github.com/IlliaVern/go-todo-app"
	"github.com/jmoiron/sqlx"
)

type TodoListPostgres struct {
	db *sqlx.DB
}

func NewTodoListPostgres(db *sqlx.DB) *TodoListPostgres {
	return &TodoListPostgres{db: db}
}

func (r *TodoListPostgres) Create(ctx context.Context, userId int, list todo.TodoList) (int, error) {
	tx, err := r.db.BeginTxx(ctx, nil)
	if err != nil {
		return 0, err
	}
	defer tx.Rollback() //nolint:errcheck // no-op after a successful Commit

	var id int
	createListQuery := fmt.Sprintf("INSERT INTO %s (title, description) VALUES($1, $2) RETURNING id", todoListsTable)
	row := tx.QueryRowContext(ctx, createListQuery, list.Title, list.Description)
	if err := row.Scan(&id); err != nil {
		return 0, err
	}

	createUserListQuery := fmt.Sprintf("INSERT INTO %s (user_id, list_id) VALUES($1, $2)", usersListsTable)
	if _, err := tx.ExecContext(ctx, createUserListQuery, userId, id); err != nil {
		return 0, err
	}

	return id, tx.Commit()
}

func (r *TodoListPostgres) GetAll(ctx context.Context, userId int) ([]todo.TodoList, error) {
	var lists []todo.TodoList

	query := fmt.Sprintf(`SELECT tl.id, tl.title, tl.description FROM %s tl
						  INNER JOIN %s ul ON tl.id = ul.list_id
						  WHERE ul.user_id = $1`,
		todoListsTable, usersListsTable)
	err := r.db.SelectContext(ctx, &lists, query, userId)

	return lists, err
}

func (r *TodoListPostgres) GetById(ctx context.Context, userId, listId int) (todo.TodoList, error) {
	var list todo.TodoList

	query := fmt.Sprintf(`SELECT tl.id, tl.title, tl.description FROM %s tl
						  INNER JOIN %s ul ON tl.id = ul.list_id
						  WHERE ul.user_id = $1 AND ul.list_id = $2`,
		todoListsTable, usersListsTable)
	err := r.db.GetContext(ctx, &list, query, userId, listId)

	return list, err
}

func (r *TodoListPostgres) Delete(ctx context.Context, userId, listId int) error {
	query := fmt.Sprintf(`DELETE FROM %s tl USING %s ul
						  WHERE tl.id = ul.list_id AND ul.user_id = $1 AND ul.list_id = $2`,
		todoListsTable, usersListsTable)
	_, err := r.db.ExecContext(ctx, query, userId, listId)

	return err
}

func (r *TodoListPostgres) Update(ctx context.Context, userId, listId int, input todo.UpdateListInput) error {
	setValues := make([]string, 0)
	args := make([]any, 0)
	argId := 1

	if input.Title != nil {
		setValues = append(setValues, fmt.Sprintf("title=$%d", argId))
		args = append(args, *input.Title)
		argId++
	}

	if input.Description != nil {
		setValues = append(setValues, fmt.Sprintf("description=$%d", argId))
		args = append(args, *input.Description)
		argId++
	}

	setQuery := strings.Join(setValues, ", ")

	query := fmt.Sprintf(`UPDATE %s tl SET %s FROM %s ul
						  WHERE tl.id = ul.list_id AND ul.list_id = $%d AND ul.user_id = $%d`,
		todoListsTable, setQuery, usersListsTable, argId, argId+1)
	args = append(args, listId, userId)

	slog.DebugContext(ctx, "update todo list", "query", query, "args", args)

	_, err := r.db.ExecContext(ctx, query, args...)
	return err
}
