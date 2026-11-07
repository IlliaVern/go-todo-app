package service

import (
	"context"

	"github.com/IlliaVern/go-todo-app"
	"github.com/IlliaVern/go-todo-app/pkg/repository"
)

type Authorization interface {
	CreateUser(ctx context.Context, user todo.User) (int, error)
	GenerateToken(ctx context.Context, username, password string) (string, error)
	ParseToken(token string) (int, error)
}

type TodoList interface {
	Create(ctx context.Context, userId int, list todo.TodoList) (int, error)
	GetAll(ctx context.Context, userId int) ([]todo.TodoList, error)
	GetById(ctx context.Context, userId, listId int) (todo.TodoList, error)
	Delete(ctx context.Context, userId, listId int) error
	Update(ctx context.Context, userId, listId int, input todo.UpdateListInput) error
}

type TodoItem interface {
	Create(ctx context.Context, userId, listId int, item todo.TodoItem) (int, error)
	GetAll(ctx context.Context, userId, listId int) ([]todo.TodoItem, error)
	GetById(ctx context.Context, userId, itemId int) (todo.TodoItem, error)
	Delete(ctx context.Context, userId, itemId int) error
	Update(ctx context.Context, userId, itemId int, input todo.UpdateItemInput) error
}

type Service struct {
	Authorization
	TodoList
	TodoItem
}

func NewService(repos *repository.Repository, signingKey string) *Service {
	return &Service{
		Authorization: NewAuthService(repos.Authorization, signingKey),
		TodoList:      NewTodoListService(repos.TodoList),
		TodoItem:      NewTodoItemService(repos.TodoItem, repos.TodoList),
	}
}
