// Package store defines the storage interfaces of Sforza and their
// implementations: a GORM-backed store (SQLite, PostgreSQL, MySQL) and a
// local JSON file store.
package store

import (
	"errors"
	"fmt"
	"sort"
	"strings"

	"github.com/FreshMag/sforza/internal/config"
	"github.com/FreshMag/sforza/internal/model"
)

// Sentinel errors returned by store implementations.
var (
	ErrNotFound = errors.New("not found")
	ErrConflict = errors.New("already exists")
)

// User is a lazily provisioned principal identified by its OIDC sub claim.
type User struct {
	Sub string `json:"sub"`
}

// Resource is a logical domain entity that groups operations.
type Resource struct {
	Name string `json:"name"`
}

// Operation is an action on a resource, named "resource:action".
type Operation struct {
	Name     string `json:"name"`
	Resource string `json:"resource"`
}

// Role is a named set of (operation, scope) assignments within a tenant.
type Role struct {
	Name string `json:"name"`
}

// RoleGrant is one (role, operation, scope) triple, used by permission
// resolution to attribute RESTRICTED grants to the roles providing them.
type RoleGrant struct {
	Role      string
	Operation string
	Scope     model.Scope
}

// Shared is the global store holding resources, operations and users.
type Shared interface {
	EnsureUser(sub string) error
	ListUsers() ([]User, error)

	CreateResource(name string) error // ErrConflict when it exists
	EnsureResource(name string) error
	DeleteResource(name string) error // ErrNotFound; cascades operations
	ListResources() ([]Resource, error)

	CreateOperation(name, resource string) error // ErrConflict; ensures the resource
	EnsureOperation(name, resource string) error
	DeleteOperation(name string) error // ErrNotFound
	ListOperations() ([]Operation, error)
	OperationExists(name string) (bool, error)
}

// Tenant is the per-tenant store holding roles, assignments, permissions
// and restricted record IDs.
type Tenant interface {
	CreateRole(name string) error // ErrConflict
	EnsureRole(name string) error
	RoleExists(name string) (bool, error)
	RenameRole(name, newName string) error // ErrNotFound / ErrConflict
	DeleteRole(name string) error          // ErrNotFound; cascades everything
	ListRoles() ([]Role, error)

	AssignRole(sub, role string) error   // ErrNotFound when the role is missing; idempotent
	UnassignRole(sub, role string) error // ErrNotFound when not assigned
	UserRoles(sub string) ([]string, error)

	SetRolePermission(role, operation string, scope model.Scope) error // upsert; ErrNotFound role
	RemoveRolePermission(role, operation string) error                 // ErrNotFound; cascades IDs
	RolePermissions(role string) ([]model.OperationScope, error)       // ErrNotFound role
	HasRolePermission(role, operation string) (bool, error)            // ErrNotFound role

	SetUserPermission(sub, operation string, scope model.Scope) error // upsert
	RemoveUserPermission(sub, operation string) error                 // ErrNotFound; cascades IDs
	UserPermissions(sub string) ([]model.OperationScope, error)
	HasUserPermission(sub, operation string) (bool, error)

	AddRoleRestrictedIDs(role, operation string, ids []string) error // idempotent
	RemoveRoleRestrictedIDs(role, operation string, ids []string) error
	RoleRestrictedIDs(roles []string, operation string) ([]string, error) // deduplicated union

	AddUserRestrictedIDs(sub, operation string, ids []string) error // idempotent
	RemoveUserRestrictedIDs(sub, operation string, ids []string) error
	UserRestrictedIDs(sub, operation string) ([]string, error)

	// RoleGrants returns every (role, operation, scope) triple of the given roles.
	RoleGrants(roles []string) ([]RoleGrant, error)
}

// Stores holds the shared store plus one store per tenant.
type Stores struct {
	Shared  Shared
	tenants map[string]Tenant
}

// Open connects every configured store and runs migrations where needed.
// Opening is idempotent, so repeated startups are safe.
func Open(cfg config.Storage) (*Stores, error) {
	shared, err := openShared(cfg.Shared)
	if err != nil {
		return nil, fmt.Errorf("open shared store: %w", err)
	}
	s := &Stores{Shared: shared, tenants: map[string]Tenant{}}
	for id, dbCfg := range cfg.Tenants {
		tenant, err := openTenant(dbCfg)
		if err != nil {
			return nil, fmt.Errorf("open tenant %q store: %w", id, err)
		}
		s.tenants[id] = tenant
	}
	return s, nil
}

// Tenant returns the store for the given tenant ID, or false when the
// tenant is not configured.
func (s *Stores) Tenant(id string) (Tenant, bool) {
	t, ok := s.tenants[id]
	return t, ok
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

func openShared(cfg config.DB) (Shared, error) {
	switch strings.ToLower(cfg.Driver) {
	case "json":
		return openJSONShared(cfg.DSN)
	case "sqlite", "postgres", "mysql":
		db, err := openGorm(cfg)
		if err != nil {
			return nil, err
		}
		if err := db.AutoMigrate(sharedModels()...); err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
		return &gormShared{db: db}, nil
	default:
		return nil, fmt.Errorf("unsupported driver %q (supported: sqlite, postgres, mysql, json)", cfg.Driver)
	}
}

func openTenant(cfg config.DB) (Tenant, error) {
	switch strings.ToLower(cfg.Driver) {
	case "json":
		return openJSONTenant(cfg.DSN)
	case "sqlite", "postgres", "mysql":
		db, err := openGorm(cfg)
		if err != nil {
			return nil, err
		}
		if err := db.AutoMigrate(tenantModels()...); err != nil {
			return nil, fmt.Errorf("migrate: %w", err)
		}
		return &gormTenant{db: db}, nil
	default:
		return nil, fmt.Errorf("unsupported driver %q (supported: sqlite, postgres, mysql, json)", cfg.Driver)
	}
}
