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

// StorageConfig returns a SQLite storage configuration rooted in a temp dir.
func StorageConfig(t *testing.T) config.Storage {
	t.Helper()
	dir := t.TempDir()
	cfg := config.Storage{
		Shared:  config.DB{Driver: "sqlite", DSN: filepath.Join(dir, "shared.db")},
		Tenants: map[string]config.DB{},
	}
	for _, id := range Tenants {
		cfg.Tenants[id] = config.DB{Driver: "sqlite", DSN: filepath.Join(dir, id+".db")}
	}
	return cfg
}

// NewStores opens migrated SQLite stores in a temp dir.
func NewStores(t *testing.T) *store.Stores {
	t.Helper()
	stores, err := store.Open(StorageConfig(t))
	if err != nil {
		t.Fatalf("open stores: %v", err)
	}
	return stores
}
