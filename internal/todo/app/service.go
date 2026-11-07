// Package app holds the todo bounded context's use cases. Every use case acts
// on behalf of an authenticated owner and only sees that owner's data.
package app

import (
	"context"

	"github.com/IlliaVern/go-todo-app/internal/todo/domain"
)

type Service struct {
	lists domain.ListRepository
	items domain.ItemRepository
}

func NewService(lists domain.ListRepository, items domain.ItemRepository) *Service {
	return &Service{lists: lists, items: items}
}

func (s *Service) CreateList(ctx context.Context, ownerID int64, title, description string) (int64, error) {
	list, err := domain.NewList(ownerID, title, description)
	if err != nil {
		return 0, err
	}
	return s.lists.Add(ctx, list)
}

func (s *Service) Lists(ctx context.Context, ownerID int64) ([]domain.List, error) {
	return s.lists.ListByOwner(ctx, ownerID)
}

func (s *Service) List(ctx context.Context, ownerID, listID int64) (domain.List, error) {
	return s.lists.Get(ctx, ownerID, listID)
}

func (s *Service) UpdateList(ctx context.Context, ownerID, listID int64, patch domain.ListPatch) error {
	list, err := s.lists.Get(ctx, ownerID, listID)
	if err != nil {
		return err
	}
	if err := list.Apply(patch); err != nil {
		return err
	}
	return s.lists.Update(ctx, list)
}

func (s *Service) DeleteList(ctx context.Context, ownerID, listID int64) error {
	return s.lists.Delete(ctx, ownerID, listID)
}

func (s *Service) AddItem(ctx context.Context, ownerID, listID int64, title, description string) (int64, error) {
	if _, err := s.lists.Get(ctx, ownerID, listID); err != nil {
		return 0, err
	}
	item, err := domain.NewItem(listID, title, description)
	if err != nil {
		return 0, err
	}
	return s.items.Add(ctx, ownerID, item)
}

func (s *Service) Items(ctx context.Context, ownerID, listID int64) ([]domain.Item, error) {
	if _, err := s.lists.Get(ctx, ownerID, listID); err != nil {
		return nil, err
	}
	return s.items.ListByList(ctx, ownerID, listID)
}

func (s *Service) Item(ctx context.Context, ownerID, itemID int64) (domain.Item, error) {
	return s.items.Get(ctx, ownerID, itemID)
}

func (s *Service) UpdateItem(ctx context.Context, ownerID, itemID int64, patch domain.ItemPatch) error {
	item, err := s.items.Get(ctx, ownerID, itemID)
	if err != nil {
		return err
	}
	if err := item.Apply(patch); err != nil {
		return err
	}
	return s.items.Update(ctx, ownerID, item)
}

func (s *Service) DeleteItem(ctx context.Context, ownerID, itemID int64) error {
	return s.items.Delete(ctx, ownerID, itemID)
}
