package domain

// Item is an aggregate root referencing the list it belongs to. It is owned by
// whoever owns that list.
type Item struct {
	id          int64
	listID      int64
	title       string
	description string
	done        bool
}

// NewItem creates an undone item that has not been persisted yet.
func NewItem(listID int64, title, description string) (Item, error) {
	if err := validateTitle(title); err != nil {
		return Item{}, err
	}
	if err := validateDescription(description); err != nil {
		return Item{}, err
	}
	return Item{listID: listID, title: title, description: description}, nil
}

// RestoreItem rebuilds a persisted item. Only repositories call it.
func RestoreItem(id, listID int64, title, description string, done bool) Item {
	return Item{id: id, listID: listID, title: title, description: description, done: done}
}

func (i Item) ID() int64           { return i.id }
func (i Item) ListID() int64       { return i.listID }
func (i Item) Title() string       { return i.title }
func (i Item) Description() string { return i.description }
func (i Item) Done() bool          { return i.done }

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

	title, description, done := i.title, i.description, i.done
	if p.Title != nil {
		title = *p.Title
	}
	if p.Description != nil {
		description = *p.Description
	}
	if p.Done != nil {
		done = *p.Done
	}

	if err := validateTitle(title); err != nil {
		return err
	}
	if err := validateDescription(description); err != nil {
		return err
	}

	i.title, i.description, i.done = title, description, done
	return nil
}
