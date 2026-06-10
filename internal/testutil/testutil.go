// Package testutil provides shared fixtures for Sforza tests.
package testutil

import (
	"path/filepath"
	"testing"

	"github.com/FreshMag/sforza/internal/config"
	"github.com/FreshMag/sforza/internal/store"
)

// AdminSub is the bootstrap administrator subject used in tests.
const AdminSub = "admin-user-sub"

// Tenants are the tenant IDs configured by NewStores.
var Tenants = []string{"tenant-a", "tenant-b"}

// Drivers are the storage backends that can be exercised without external
// services.
var Drivers = []string{"sqlite", "json"}

// StorageConfig returns an SQLite storage configuration rooted in a temp dir.
func StorageConfig(t *testing.T) config.Storage {
	return StorageConfigDriver(t, "sqlite")
}

// StorageConfigDriver returns a storage configuration for the given driver,
// rooted in a temp dir.
func StorageConfigDriver(t *testing.T, driver string) config.Storage {
	t.Helper()
	dir := t.TempDir()
	ext := map[string]string{"sqlite": ".db", "json": ".json"}[driver]
	cfg := config.Storage{
		Shared:  config.DB{Driver: driver, DSN: filepath.Join(dir, "shared"+ext)},
		Tenants: map[string]config.DB{},
	}
	for _, id := range Tenants {
		cfg.Tenants[id] = config.DB{Driver: driver, DSN: filepath.Join(dir, id+ext)}
	}
	return cfg
}

// NewStores opens migrated SQLite stores in a temp dir.
func NewStores(t *testing.T) *store.Stores {
	return NewStoresDriver(t, "sqlite")
}

// NewStoresDriver opens stores backed by the given driver in a temp dir.
func NewStoresDriver(t *testing.T, driver string) *store.Stores {
	t.Helper()
	stores, err := store.Open(StorageConfigDriver(t, driver))
	if err != nil {
		t.Fatalf("open %s stores: %v", driver, err)
	}
	return stores
}
