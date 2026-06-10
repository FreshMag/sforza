// Package service implements SFBAC permission resolution, administration
// and bootstrap synchronization on top of the store layer.
package service

import "errors"

// Sentinel errors mapped to HTTP status codes by the API layer.
var (
	ErrNotFound   = errors.New("not found")
	ErrConflict   = errors.New("already exists")
	ErrValidation = errors.New("validation error")
)
