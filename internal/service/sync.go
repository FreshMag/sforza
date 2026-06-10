package service

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"gopkg.in/yaml.v3"

	"github.com/FreshMag/sforza/internal/model"
	"github.com/FreshMag/sforza/internal/store"
)

// BootstrapFile is the YAML document a microservice contributes to register
// its resources, operations and tenant configuration.
//
//	resources:
//	  - product
//	operations:
//	  - product:read
//	  - product:write
//	tenants:
//	  tenant-a:
//	    roles:
//	      manager:
//	        product:read: FULL
//	        product:write:
//	          scope: RESTRICTED
//	          ids: [10, 15]
//	    users:
//	      john:
//	        roles: [manager]
//	        permissions:
//	          product:read: FULL
type BootstrapFile struct {
	Resources  []string                   `yaml:"resources"`
	Operations []string                   `yaml:"operations"`
	Tenants    map[string]TenantBootstrap `yaml:"tenants"`
}

// TenantBootstrap declares roles and users for one tenant.
type TenantBootstrap struct {
	Roles map[string]map[string]PermissionSpec `yaml:"roles"`
	Users map[string]UserBootstrap             `yaml:"users"`
}

// UserBootstrap declares role assignments and permission overrides for a user.
type UserBootstrap struct {
	Roles       []string                  `yaml:"roles"`
	Permissions map[string]PermissionSpec `yaml:"permissions"`
}

// PermissionSpec is either a bare scope ("FULL") or a mapping with a scope
// and optional restricted IDs.
type PermissionSpec struct {
	Scope model.Scope
	IDs   []string
}

// UnmarshalYAML accepts both the scalar and the mapping form.
func (p *PermissionSpec) UnmarshalYAML(node *yaml.Node) error {
	if node.Kind == yaml.ScalarNode {
		p.Scope = model.Scope(node.Value)
		return nil
	}
	var aux struct {
		Scope model.Scope `yaml:"scope"`
		IDs   []any       `yaml:"ids"`
	}
	if err := node.Decode(&aux); err != nil {
		return err
	}
	p.Scope = aux.Scope
	for _, id := range aux.IDs {
		p.IDs = append(p.IDs, fmt.Sprint(id))
	}
	return nil
}

func (p PermissionSpec) validate(operation string) error {
	if !p.Scope.Valid() {
		return fmt.Errorf("%w: operation %q has invalid scope %q", ErrValidation, operation, p.Scope)
	}
	return nil
}

// LoadBootstrapFiles reads every file matched by the given glob patterns and
// parses it as a BootstrapFile.
func LoadBootstrapFiles(patterns []string) ([]BootstrapFile, error) {
	var files []BootstrapFile
	for _, pattern := range patterns {
		matches, err := filepath.Glob(pattern)
		if err != nil {
			return nil, fmt.Errorf("bad bootstrap pattern %q: %w", pattern, err)
		}
		sort.Strings(matches)
		for _, path := range matches {
			raw, err := os.ReadFile(path)
			if err != nil {
				return nil, fmt.Errorf("read bootstrap file %q: %w", path, err)
			}
			var bf BootstrapFile
			if err := yaml.Unmarshal(raw, &bf); err != nil {
				return nil, fmt.Errorf("parse bootstrap file %q: %w", path, err)
			}
			files = append(files, bf)
		}
	}
	return files, nil
}

// Sync applies bootstrap files to the databases. It is additive and
// idempotent: declared entities are created or updated to match, while
// entities not mentioned are left untouched (other services may own them).
func Sync(stores *store.Stores, files []BootstrapFile) error {
	for _, f := range files {
		if err := syncFile(stores, f); err != nil {
			return err
		}
	}
	return nil
}

func syncFile(stores *store.Stores, f BootstrapFile) error {
	for _, name := range f.Resources {
		if err := stores.Shared.Where(&store.Resource{Name: name}).
			FirstOrCreate(&store.Resource{Name: name}).Error; err != nil {
			return fmt.Errorf("sync resource %q: %w", name, err)
		}
	}
	for _, name := range f.Operations {
		if err := EnsureOperation(stores.Shared, name); err != nil {
			return fmt.Errorf("sync operation %q: %w", name, err)
		}
	}
	for tenantID, tb := range f.Tenants {
		tenant, ok := stores.Tenant(tenantID)
		if !ok {
			return fmt.Errorf("%w: bootstrap references tenant %q which is not configured", ErrValidation, tenantID)
		}
		for roleName, perms := range tb.Roles {
			if _, err := EnsureRole(tenant, roleName); err != nil {
				return fmt.Errorf("tenant %q: sync role %q: %w", tenantID, roleName, err)
			}
			for op, spec := range perms {
				if err := spec.validate(op); err != nil {
					return err
				}
				// Operations referenced by permissions are registered too, so
				// a single file stays self-contained.
				if err := EnsureOperation(stores.Shared, op); err != nil {
					return err
				}
				if err := SetRolePermission(stores.Shared, tenant, roleName, op, spec.Scope); err != nil {
					return fmt.Errorf("tenant %q: role %q: set %q: %w", tenantID, roleName, op, err)
				}
				if len(spec.IDs) > 0 {
					if err := UpdateRoleRestrictedIDs(tenant, roleName, op, spec.IDs, nil); err != nil {
						return fmt.Errorf("tenant %q: role %q: ids for %q: %w", tenantID, roleName, op, err)
					}
				}
			}
		}
		for sub, ub := range tb.Users {
			if err := EnsureUser(stores.Shared, sub); err != nil {
				return fmt.Errorf("tenant %q: sync user %q: %w", tenantID, sub, err)
			}
			for _, roleName := range ub.Roles {
				if err := AssignRole(stores.Shared, tenant, sub, roleName); err != nil {
					return fmt.Errorf("tenant %q: assign role %q to %q: %w", tenantID, roleName, sub, err)
				}
			}
			for op, spec := range ub.Permissions {
				if err := spec.validate(op); err != nil {
					return err
				}
				if err := EnsureOperation(stores.Shared, op); err != nil {
					return err
				}
				if err := SetUserPermission(stores.Shared, tenant, sub, op, spec.Scope); err != nil {
					return fmt.Errorf("tenant %q: user %q: set %q: %w", tenantID, sub, op, err)
				}
				if len(spec.IDs) > 0 {
					if err := UpdateUserRestrictedIDs(tenant, sub, op, spec.IDs, nil); err != nil {
						return fmt.Errorf("tenant %q: user %q: ids for %q: %w", tenantID, sub, op, err)
					}
				}
			}
		}
	}
	return nil
}

// BootstrapMeta registers Sforza's own meta resources and operations in the
// shared database, then ensures the administrator role exists in every
// tenant with FULL scope on every meta operation and assigns it to adminSub.
func BootstrapMeta(stores *store.Stores, adminSub string) error {
	metaOps := make([]string, 0, len(model.MetaOperations))
	for op := range model.MetaOperations {
		metaOps = append(metaOps, op)
	}
	sort.Strings(metaOps)

	for _, op := range metaOps {
		if err := EnsureOperation(stores.Shared, op); err != nil {
			return fmt.Errorf("bootstrap meta operation %q: %w", op, err)
		}
	}
	if err := EnsureUser(stores.Shared, adminSub); err != nil {
		return fmt.Errorf("bootstrap admin user: %w", err)
	}
	for _, tenantID := range stores.TenantIDs() {
		tenant, _ := stores.Tenant(tenantID)
		if _, err := EnsureRole(tenant, model.AdminRole); err != nil {
			return fmt.Errorf("tenant %q: bootstrap admin role: %w", tenantID, err)
		}
		for _, op := range metaOps {
			if err := SetRolePermission(stores.Shared, tenant, model.AdminRole, op, model.ScopeFull); err != nil {
				return fmt.Errorf("tenant %q: bootstrap admin permission %q: %w", tenantID, op, err)
			}
		}
		if err := AssignRole(stores.Shared, tenant, adminSub, model.AdminRole); err != nil {
			return fmt.Errorf("tenant %q: bootstrap admin assignment: %w", tenantID, err)
		}
	}
	return nil
}
