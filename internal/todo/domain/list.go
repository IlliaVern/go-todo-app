// Package domain is the todo bounded context's model: lists owned by a user
// and the items inside them, with their invariants and repository ports.
package domain

// List is an aggregate root: a titled collection of items owned by one user.
type List struct {
	id          ListID
	ownerID     OwnerID
	title       Title
	description Description
	// version increases on every saved update; repositories use it for
	// optimistic concurrency control.
	version int64
}

func NewList(owner OwnerID, title Title, description Description) List {
	return List{ownerID: owner, title: title, description: description}
}

// RestoreList rebuilds a persisted list from trusted storage without
// re-validating it. Only repositories call it.
func RestoreList(id ListID, owner OwnerID, title, description string, version int64) List {
	return List{id: id, ownerID: owner, title: Title{title}, description: Description{description}, version: version}
}

func (l List) ID() ListID               { return l.id }
func (l List) OwnerID() OwnerID         { return l.ownerID }
func (l List) Title() Title             { return l.title }
func (l List) Description() Description { return l.description }
func (l List) Version() int64           { return l.version }

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

	title, description := l.title.String(), l.description.String()
	if p.Title != nil {
		title = *p.Title
	}
	if p.Description != nil {
		description = *p.Description
	}

	t, d, err := ParseText(title, description)
	if err != nil {
		return err
	}
	l.title, l.description = t, d
	return nil
}
