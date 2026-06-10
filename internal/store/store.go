// Package store opens and migrates the shared and per-tenant databases.
package store

import (
	"fmt"
	"sort"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"

	"github.com/francesco/sforza/internal/config"
)

// Stores holds the shared database plus one database handle per tenant.
type Stores struct {
	Shared  *gorm.DB
	tenants map[string]*gorm.DB
}

// Open connects to every configured database and runs migrations. Migration
// is idempotent, so repeated startups are safe.
func Open(cfg config.Storage) (*Stores, error) {
	shared, err := open(cfg.Shared)
	if err != nil {
		return nil, fmt.Errorf("open shared database: %w", err)
	}
	if err := shared.AutoMigrate(sharedModels()...); err != nil {
		return nil, fmt.Errorf("migrate shared database: %w", err)
	}
	s := &Stores{Shared: shared, tenants: map[string]*gorm.DB{}}
	for id, dbCfg := range cfg.Tenants {
		db, err := open(dbCfg)
		if err != nil {
			return nil, fmt.Errorf("open tenant %q database: %w", id, err)
		}
		if err := db.AutoMigrate(tenantModels()...); err != nil {
			return nil, fmt.Errorf("migrate tenant %q database: %w", id, err)
		}
		s.tenants[id] = db
	}
	return s, nil
}

// Tenant returns the database for the given tenant ID, or false when the
// tenant is not configured.
func (s *Stores) Tenant(id string) (*gorm.DB, bool) {
	db, ok := s.tenants[id]
	return db, ok
}

// TenantIDs returns the sorted list of configured tenant IDs.
func (s *Stores) TenantIDs() []string {
	ids := make([]string, 0, len(s.tenants))
	for id := range s.tenants {
		ids = append(ids, id)
	}
	sort.Strings(ids)
	return ids
}

func open(cfg config.DB) (*gorm.DB, error) {
	gormCfg := &gorm.Config{Logger: logger.Default.LogMode(logger.Silent)}
	switch strings.ToLower(cfg.Driver) {
	case "sqlite":
		db, err := gorm.Open(sqlite.Open(cfg.DSN), gormCfg)
		if err != nil {
			return nil, err
		}
		// SQLite supports a single writer; serializing connections avoids
		// SQLITE_BUSY errors under concurrent API calls.
		sqlDB, err := db.DB()
		if err != nil {
			return nil, err
		}
		sqlDB.SetMaxOpenConns(1)
		return db, nil
	case "postgres":
		return gorm.Open(postgres.Open(cfg.DSN), gormCfg)
	default:
		return nil, fmt.Errorf("unsupported driver %q (supported: sqlite, postgres)", cfg.Driver)
	}
}
