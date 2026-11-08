package domain

// Item is an aggregate root referencing the list it belongs to. It is owned by
// whoever owns that list.
type Item struct {
	id          ItemID
	listID      ListID
	title       Title
	description Description
	done        bool
	version     int64 // see List.version
}

func NewItem(list ListID, title Title, description Description) Item {
	return Item{listID: list, title: title, description: description}
}

// RestoreItem rebuilds a persisted item from trusted storage without
// re-validating it. Only repositories call it.
func RestoreItem(id ItemID, list ListID, title, description string, done bool, version int64) Item {
	return Item{id: id, listID: list, title: Title{title}, description: Description{description}, done: done, version: version}
}

func (i Item) ID() ItemID               { return i.id }
func (i Item) ListID() ListID           { return i.listID }
func (i Item) Title() Title             { return i.title }
func (i Item) Description() Description { return i.description }
func (i Item) Done() bool               { return i.done }
func (i Item) Version() int64           { return i.version }

// ItemPatch is a partial update; nil fields stay unchanged.
type ItemPatch struct {
	Title       *string
	Description *string
	Done        *bool
}

// Apply updates the item from p, or leaves it untouched and returns a
// ValidationError when p is empty or the result would break an invariant.
func (i *Item) Apply(p ItemPatch) error {
	if p.Title == nil && p.Description == nil && p.Done == nil {
		return ErrEmptyUpdate
	}

	title, description, done := i.title.String(), i.description.String(), i.done
	if p.Title != nil {
		title = *p.Title
	}
	if p.Description != nil {
		description = *p.Description
	}
	if p.Done != nil {
		done = *p.Done
	}

	t, d, err := ParseText(TextInput{Title: title, Description: description})
	if err != nil {
		return err
	}
	i.title, i.description, i.done = t, d, done
	return nil
}
