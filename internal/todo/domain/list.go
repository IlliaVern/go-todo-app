// Package domain is the todo bounded context's model: lists owned by a user
// and the items inside them, with their invariants and repository ports.
package domain

// List is an aggregate root: a titled collection of items owned by one user.
type List struct {
	id          int64
	ownerID     int64
	title       string
	description string
	// version increases on every saved update; repositories use it for
	// optimistic concurrency control.
	version int64
}

// NewList creates a list that has not been persisted yet.
func NewList(ownerID int64, title, description string) (List, error) {
	if err := validateTitle(title); err != nil {
		return List{}, err
	}
	if err := validateDescription(description); err != nil {
		return List{}, err
	}
	return List{ownerID: ownerID, title: title, description: description}, nil
}

// RestoreList rebuilds a persisted list. Only repositories call it.
func RestoreList(id, ownerID int64, title, description string, version int64) List {
	return List{id: id, ownerID: ownerID, title: title, description: description, version: version}
}

func (l List) ID() int64           { return l.id }
func (l List) OwnerID() int64      { return l.ownerID }
func (l List) Title() string       { return l.title }
func (l List) Description() string { return l.description }
func (l List) Version() int64      { return l.version }

// ListPatch is a partial update; nil fields stay unchanged.
type ListPatch struct {
	Title       *string
	Description *string
}

// Apply updates the list from p, or leaves it untouched and returns a
// ValidationError when p is empty or the result would break an invariant.
func (l *List) Apply(p ListPatch) error {
	if p.Title == nil && p.Description == nil {
		return ErrEmptyUpdate
	}

	title, description := l.title, l.description
	if p.Title != nil {
		title = *p.Title
	}
	if p.Description != nil {
		description = *p.Description
	}

	if err := validateTitle(title); err != nil {
		return err
	}
	if err := validateDescription(description); err != nil {
		return err
	}

	l.title, l.description = title, description
	return nil
}
