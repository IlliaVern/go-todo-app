package service

import (
	"context"

	"github.com/IlliaVern/go-todo-app"
	"github.com/IlliaVern/go-todo-app/pkg/repository"
)

type TodoListService struct {
	repo repository.TodoList
}

func NewTodoListService(repo repository.TodoList) *TodoListService {
	return &TodoListService{repo: repo}
}

func (s *TodoListService) Create(ctx context.Context, userId int, list todo.TodoList) (int, error) {
	return s.repo.Create(ctx, userId, list)
}

func (s *TodoListService) GetAll(ctx context.Context, userId int) ([]todo.TodoList, error) {
	return s.repo.GetAll(ctx, userId)
}

func (s *TodoListService) GetById(ctx context.Context, userId, listId int) (todo.TodoList, error) {
	return s.repo.GetById(ctx, userId, listId)
}

func (s *TodoListService) Delete(ctx context.Context, userId, listId int) error {
	return s.repo.Delete(ctx, userId, listId)
}

func (s *TodoListService) Update(ctx context.Context, userId, listId int, input todo.UpdateListInput) error {
	if err := input.Validate(); err != nil {
		return err
	}
	return s.repo.Update(ctx, userId, listId, input)
}
