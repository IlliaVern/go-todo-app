package service

import (
	"context"

	"github.com/IlliaVern/go-todo-app"
	"github.com/IlliaVern/go-todo-app/pkg/repository"
)

type TodoItemService struct {
	repo     repository.TodoItem
	listRepo repository.TodoList
}

func NewTodoItemService(repo repository.TodoItem, listRepo repository.TodoList) *TodoItemService {
	return &TodoItemService{repo: repo, listRepo: listRepo}
}

func (s *TodoItemService) Create(ctx context.Context, userId, listId int, item todo.TodoItem) (int, error) {
	_, err := s.listRepo.GetById(ctx, userId, listId)
	if err != nil {
		return 0, err
	}

	return s.repo.Create(ctx, listId, item)
}

func (s *TodoItemService) GetAll(ctx context.Context, userId, listId int) ([]todo.TodoItem, error) {
	return s.repo.GetAll(ctx, userId, listId)
}

func (s *TodoItemService) GetById(ctx context.Context, userId, itemId int) (todo.TodoItem, error) {
	return s.repo.GetById(ctx, userId, itemId)
}

func (s *TodoItemService) Delete(ctx context.Context, userId, itemId int) error {
	return s.repo.Delete(ctx, userId, itemId)
}

func (s *TodoItemService) Update(ctx context.Context, userId, itemId int, input todo.UpdateItemInput) error {
	if err := input.Validate(); err != nil {
		return err
	}
	return s.repo.Update(ctx, userId, itemId, input)
}
