// Package service implements SFBAC permission resolution, administration
// and bootstrap synchronization on top of the store layer.
package service

import (
	"errors"

	"github.com/FreshMag/sforza/internal/store"
)

// Sentinel errors mapped to HTTP status codes by the API layer. ErrNotFound
// and ErrConflict alias the store's sentinels so errors.Is works across
// layers.
var (
	ErrNotFound   = store.ErrNotFound
	ErrConflict   = store.ErrConflict
	ErrValidation = errors.New("validation error")
)
