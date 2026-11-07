package domain

import "context"

// ListRepository persists lists. Every lookup is scoped to the owner: a list
// owned by someone else is reported as ErrListNotFound.
type ListRepository interface {
	// Add stores a new list and returns its id, or ErrOwnerNotFound when the
	// owner no longer exists.
	Add(ctx context.Context, list List) (int64, error)
	Get(ctx context.Context, ownerID, listID int64) (List, error)
	ListByOwner(ctx context.Context, ownerID int64) ([]List, error)
	// Update saves title and description of a list previously returned by Get.
	Update(ctx context.Context, list List) error
	// Delete removes the list together with its items.
	Delete(ctx context.Context, ownerID, listID int64) error
}

// ItemRepository persists items. Every lookup is scoped to the owner of the
// item's list: an item in someone else's list is reported as ErrItemNotFound.
type ItemRepository interface {
	// Add stores a new item and returns its id, or ErrListNotFound when its
	// list does not exist or is not owned by ownerID.
	Add(ctx context.Context, ownerID int64, item Item) (int64, error)
	Get(ctx context.Context, ownerID, itemID int64) (Item, error)
	ListByList(ctx context.Context, ownerID, listID int64) ([]Item, error)
	// Update saves title, description and done of an item previously returned by Get.
	Update(ctx context.Context, ownerID int64, item Item) error
	Delete(ctx context.Context, ownerID, itemID int64) error
}
