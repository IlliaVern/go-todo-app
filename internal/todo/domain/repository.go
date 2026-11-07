package domain

import "context"

// ListRepository persists lists. Every lookup is scoped to the owner: a list
// owned by someone else is reported as ErrListNotFound.
type ListRepository interface {
	// Add stores a new list and returns its id, or ErrOwnerNotFound.
	Add(ctx context.Context, list List) (ListID, error)
	Get(ctx context.Context, owner OwnerID, id ListID) (List, error)
	ByOwner(ctx context.Context, owner OwnerID) ([]List, error)
	// Update saves title and description of a list previously returned by Get
	// and returns it with its new version, or ErrConcurrentUpdate when it
	// changed since it was loaded.
	Update(ctx context.Context, list List) (List, error)
	// Delete removes the list together with its items.
	Delete(ctx context.Context, owner OwnerID, id ListID) error
}

// ItemRepository persists items. Every lookup is scoped to the owner of the
// item's list: an item in someone else's list is reported as ErrItemNotFound.
type ItemRepository interface {
	// Add stores a new item and returns its id, or ErrListNotFound when its
	// list does not exist or is not owned by owner.
	Add(ctx context.Context, owner OwnerID, item Item) (ItemID, error)
	Get(ctx context.Context, owner OwnerID, id ItemID) (Item, error)
	ByList(ctx context.Context, owner OwnerID, list ListID) ([]Item, error)
	// Update saves title, description and done of an item previously returned
	// by Get and returns it with its new version, or ErrConcurrentUpdate when
	// it changed since it was loaded.
	Update(ctx context.Context, owner OwnerID, item Item) (Item, error)
	Delete(ctx context.Context, owner OwnerID, id ItemID) error
}
