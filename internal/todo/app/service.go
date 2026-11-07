// Package app holds the todo bounded context's use cases. Every use case acts
// on behalf of an authenticated owner and only sees that owner's data.
package app

import (
	"context"
	"errors"
	"math/rand/v2"
	"time"

	"github.com/IlliaVern/go-todo-app/internal/todo/domain"
)

type Service struct {
	lists domain.ListRepository
	items domain.ItemRepository
}

func NewService(lists domain.ListRepository, items domain.ItemRepository) *Service {
	return &Service{lists: lists, items: items}
}

func (s *Service) CreateList(ctx context.Context, owner domain.OwnerID, title, description string) (domain.ListID, error) {
	t, d, err := domain.ParseText(title, description)
	if err != nil {
		return 0, err
	}
	return s.lists.Add(ctx, domain.NewList(owner, t, d))
}

func (s *Service) Lists(ctx context.Context, owner domain.OwnerID) ([]domain.List, error) {
	return s.lists.ByOwner(ctx, owner)
}

func (s *Service) List(ctx context.Context, owner domain.OwnerID, id domain.ListID) (domain.List, error) {
	return s.lists.Get(ctx, owner, id)
}

func (s *Service) UpdateList(ctx context.Context, owner domain.OwnerID, id domain.ListID, patch domain.ListPatch) error {
	return retryOnConflict(ctx, func() error {
		list, err := s.lists.Get(ctx, owner, id)
		if err != nil {
			return err
		}
		if err := list.Apply(patch); err != nil {
			return err
		}
		_, err = s.lists.Update(ctx, list)
		return err
	})
}

func (s *Service) DeleteList(ctx context.Context, owner domain.OwnerID, id domain.ListID) error {
	return s.lists.Delete(ctx, owner, id)
}

// AddItem relies on the repository to reject lists the owner does not have.
func (s *Service) AddItem(ctx context.Context, owner domain.OwnerID, list domain.ListID, title, description string) (domain.ItemID, error) {
	t, d, err := domain.ParseText(title, description)
	if err != nil {
		return 0, err
	}
	return s.items.Add(ctx, owner, domain.NewItem(list, t, d))
}

// Items distinguishes a missing list (ErrListNotFound) from an empty one.
func (s *Service) Items(ctx context.Context, owner domain.OwnerID, list domain.ListID) ([]domain.Item, error) {
	if _, err := s.lists.Get(ctx, owner, list); err != nil {
		return nil, err
	}
	return s.items.ByList(ctx, owner, list)
}

func (s *Service) Item(ctx context.Context, owner domain.OwnerID, id domain.ItemID) (domain.Item, error) {
	return s.items.Get(ctx, owner, id)
}

func (s *Service) UpdateItem(ctx context.Context, owner domain.OwnerID, id domain.ItemID, patch domain.ItemPatch) error {
	return retryOnConflict(ctx, func() error {
		item, err := s.items.Get(ctx, owner, id)
		if err != nil {
			return err
		}
		if err := item.Apply(patch); err != nil {
			return err
		}
		_, err = s.items.Update(ctx, owner, item)
		return err
	})
}

func (s *Service) DeleteItem(ctx context.Context, owner domain.OwnerID, id domain.ItemID) error {
	return s.items.Delete(ctx, owner, id)
}

// MaxUpdateAttempts bounds how often an update is re-applied to a freshly
// loaded aggregate after losing an optimistic concurrency race. Contention
// beyond that surfaces as ErrConcurrentUpdate.
const MaxUpdateAttempts = 5

// retryBaseDelay is the first backoff step; each retry doubles it, with jitter.
const retryBaseDelay = 2 * time.Millisecond

// retryOnConflict runs a load-apply-save cycle again when the save reports
// ErrConcurrentUpdate, so concurrent patches to different fields all land.
// Randomised exponential backoff spreads competing writers apart.
func retryOnConflict(ctx context.Context, update func() error) error {
	var err error
	for attempt := range MaxUpdateAttempts {
		if attempt > 0 {
			step := retryBaseDelay << (attempt - 1)
			wait := step + rand.N(step) //nolint:gosec // jitter, not a secret
			select {
			case <-time.After(wait):
			case <-ctx.Done():
				return ctx.Err()
			}
		}
		if err = update(); !errors.Is(err, domain.ErrConcurrentUpdate) {
			return err
		}
	}
	return err
}
