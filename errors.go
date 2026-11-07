package todo

import "errors"

// ErrNotFound is returned by repositories when the requested entity does not exist.
var ErrNotFound = errors.New("not found")
