package db

import "errors"

// ErrNotFound is returned by accessors when a row does not exist.
var ErrNotFound = errors.New("db: not found")
