package repository

import (
	"context"

	"github.com/IlliaVern/go-todo-app"
	"github.com/jmoiron/sqlx"
)

type Authorization interface {
	CreateUser(ctx context.Context, user todo.User) (int, error)
	GetUser(ctx context.Context, username string) (todo.User, error)
}

type TodoList interface {
	Create(ctx context.Context, userId int, list todo.TodoList) (int, error)
	GetAll(ctx context.Context, userId int) ([]todo.TodoList, error)
	GetById(ctx context.Context, userId, listId int) (todo.TodoList, error)
	Delete(ctx context.Context, userId, listId int) error
	Update(ctx context.Context, userId, listId int, input todo.UpdateListInput) error
}

type TodoItem interface {
	Create(ctx context.Context, listId int, item todo.TodoItem) (int, error)
	GetAll(ctx context.Context, userId, listId int) ([]todo.TodoItem, error)
	GetById(ctx context.Context, userId, itemId int) (todo.TodoItem, error)
	Delete(ctx context.Context, userId, itemId int) error
	Update(ctx context.Context, userId, itemId int, input todo.UpdateItemInput) error
}

type Repository struct {
	Authorization
	TodoList
	TodoItem
}

func NewRepository(db *sqlx.DB) *Repository {
	return &Repository{
		Authorization: NewAuthPostgres(db),
		TodoList:      NewTodoListPostgres(db),
		TodoItem:      NewTodoItemPostgres(db),
	}
}
