package service

import (
	"fmt"

	"github.com/FreshMag/sforza/internal/model"
	"github.com/FreshMag/sforza/internal/store"
)

// The admin functions validate input and enforce cross-store invariants
// (operations must be registered before being granted, users are lazily
// provisioned); persistence semantics live in the store implementations.

// --- Shared store: resources, operations, users ---

// EnsureUser lazily provisions a user in the shared store.
func EnsureUser(shared store.Shared, sub string) error {
	if sub == "" {
		return fmt.Errorf("%w: empty user sub", ErrValidation)
	}
	return shared.EnsureUser(sub)
}

// ListUsers returns all provisioned users.
func ListUsers(shared store.Shared) ([]store.User, error) {
	return shared.ListUsers()
}

// CreateResource registers a resource; creating an existing one is an error.
func CreateResource(shared store.Shared, name string) error {
	if name == "" {
		return fmt.Errorf("%w: resource name is required", ErrValidation)
	}
	return shared.CreateResource(name)
}

// DeleteResource removes a resource together with its operations.
func DeleteResource(shared store.Shared, name string) error {
	return shared.DeleteResource(name)
}

// ListResources returns all registered resources.
func ListResources(shared store.Shared) ([]store.Resource, error) {
	return shared.ListResources()
}

// CreateOperation registers an operation named "resource:action", creating
// the parent resource when missing.
func CreateOperation(shared store.Shared, name string) error {
	resource := model.OperationResource(name)
	if resource == "" {
		return fmt.Errorf("%w: operation name must have the form \"resource:action\"", ErrValidation)
	}
	return shared.CreateOperation(name, resource)
}

// EnsureOperation registers an operation if missing (idempotent variant used
// by bootstrap synchronization).
func EnsureOperation(shared store.Shared, name string) error {
	resource := model.OperationResource(name)
	if resource == "" {
		return fmt.Errorf("%w: operation name must have the form \"resource:action\"", ErrValidation)
	}
	return shared.EnsureOperation(name, resource)
}

// DeleteOperation removes an operation.
func DeleteOperation(shared store.Shared, name string) error {
	return shared.DeleteOperation(name)
}

// ListOperations returns all registered operations.
func ListOperations(shared store.Shared) ([]store.Operation, error) {
	return shared.ListOperations()
}

func requireOperation(shared store.Shared, name string) error {
	ok, err := shared.OperationExists(name)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("operation %q: %w", name, ErrNotFound)
	}
	return nil
}

// --- Tenant store: roles, assignments, permissions, restricted IDs ---

// CreateRole creates a role in the tenant.
func CreateRole(tenant store.Tenant, name string) error {
	if name == "" {
		return fmt.Errorf("%w: role name is required", ErrValidation)
	}
	return tenant.CreateRole(name)
}

// EnsureRole creates a role if missing.
func EnsureRole(tenant store.Tenant, name string) error {
	if name == "" {
		return fmt.Errorf("%w: role name is required", ErrValidation)
	}
	return tenant.EnsureRole(name)
}

// GetRole returns a role by name.
func GetRole(tenant store.Tenant, name string) (*store.Role, error) {
	ok, err := tenant.RoleExists(name)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, fmt.Errorf("role %q: %w", name, ErrNotFound)
	}
	return &store.Role{Name: name}, nil
}

// RenameRole changes a role's name.
func RenameRole(tenant store.Tenant, name, newName string) error {
	if newName == "" {
		return fmt.Errorf("%w: new role name is required", ErrValidation)
	}
	return tenant.RenameRole(name, newName)
}

// DeleteRole removes a role with all its permissions, restricted IDs and
// user assignments.
func DeleteRole(tenant store.Tenant, name string) error {
	return tenant.DeleteRole(name)
}

// ListRoles returns all roles in the tenant.
func ListRoles(tenant store.Tenant) ([]store.Role, error) {
	return tenant.ListRoles()
}

// RolePermissions returns the (operation, scope) pairs assigned to a role.
func RolePermissions(tenant store.Tenant, name string) ([]model.OperationScope, error) {
	return tenant.RolePermissions(name)
}

// AssignRole assigns a role to a user (idempotent) and lazily provisions the
// user in the shared store.
func AssignRole(shared store.Shared, tenant store.Tenant, sub, roleName string) error {
	if err := EnsureUser(shared, sub); err != nil {
		return err
	}
	return tenant.AssignRole(sub, roleName)
}

// UnassignRole removes a role from a user.
func UnassignRole(tenant store.Tenant, sub, roleName string) error {
	return tenant.UnassignRole(sub, roleName)
}

// UserRoles returns the names of the roles assigned to a user.
func UserRoles(tenant store.Tenant, sub string) ([]string, error) {
	return tenant.UserRoles(sub)
}

// SetRolePermission upserts (operation, scope) on a role. The operation must
// be registered in the shared store.
func SetRolePermission(shared store.Shared, tenant store.Tenant, roleName, operation string, scope model.Scope) error {
	if !scope.Valid() {
		return fmt.Errorf("%w: invalid scope %q", ErrValidation, scope)
	}
	if err := requireOperation(shared, operation); err != nil {
		return err
	}
	return tenant.SetRolePermission(roleName, operation, scope)
}

// RemoveRolePermission deletes (operation, scope) from a role together with
// the operation's restricted IDs.
func RemoveRolePermission(tenant store.Tenant, roleName, operation string) error {
	return tenant.RemoveRolePermission(roleName, operation)
}

// SetUserPermission upserts a user-level (operation, scope) override and
// lazily provisions the user.
func SetUserPermission(shared store.Shared, tenant store.Tenant, sub, operation string, scope model.Scope) error {
	if !scope.Valid() {
		return fmt.Errorf("%w: invalid scope %q", ErrValidation, scope)
	}
	if err := requireOperation(shared, operation); err != nil {
		return err
	}
	if err := EnsureUser(shared, sub); err != nil {
		return err
	}
	return tenant.SetUserPermission(sub, operation, scope)
}

// RemoveUserPermission deletes a user-level override together with the
// operation's restricted IDs.
func RemoveUserPermission(tenant store.Tenant, sub, operation string) error {
	return tenant.RemoveUserPermission(sub, operation)
}

// UpdateRoleRestrictedIDs adds and removes record IDs for a role's
// permission on an operation. The permission must already be assigned.
func UpdateRoleRestrictedIDs(tenant store.Tenant, roleName, operation string, add, remove []string) error {
	ok, err := tenant.HasRolePermission(roleName, operation)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("role %q has no permission on %q: %w", roleName, operation, ErrNotFound)
	}
	if err := tenant.AddRoleRestrictedIDs(roleName, operation, add); err != nil {
		return err
	}
	return tenant.RemoveRoleRestrictedIDs(roleName, operation, remove)
}

// UpdateUserRestrictedIDs adds and removes record IDs for a user's
// permission on an operation. The permission must already be assigned.
func UpdateUserRestrictedIDs(tenant store.Tenant, sub, operation string, add, remove []string) error {
	ok, err := tenant.HasUserPermission(sub, operation)
	if err != nil {
		return err
	}
	if !ok {
		return fmt.Errorf("user %q has no permission on %q: %w", sub, operation, ErrNotFound)
	}
	if err := tenant.AddUserRestrictedIDs(sub, operation, add); err != nil {
		return err
	}
	return tenant.RemoveUserRestrictedIDs(sub, operation, remove)
}
