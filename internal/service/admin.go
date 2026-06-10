package service

import (
	"errors"
	"fmt"
	"sort"

	"gorm.io/gorm"
	"gorm.io/gorm/clause"

	"github.com/FreshMag/sforza/internal/model"
	"github.com/FreshMag/sforza/internal/store"
)

// --- Shared database: resources, operations, users ---

// EnsureUser lazily provisions a user in the shared database.
func EnsureUser(shared *gorm.DB, sub string) error {
	if sub == "" {
		return fmt.Errorf("%w: empty user sub", ErrValidation)
	}
	return shared.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&store.User{Sub: sub}).Error
}

// ListUsers returns all provisioned users.
func ListUsers(shared *gorm.DB) ([]store.User, error) {
	var users []store.User
	return users, shared.Order("sub").Find(&users).Error
}

// CreateResource registers a resource; creating an existing one is an error.
func CreateResource(shared *gorm.DB, name string) error {
	if name == "" {
		return fmt.Errorf("%w: resource name is required", ErrValidation)
	}
	res := shared.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&store.Resource{Name: name})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("resource %q: %w", name, ErrConflict)
	}
	return nil
}

// DeleteResource removes a resource together with its operations.
func DeleteResource(shared *gorm.DB, name string) error {
	return shared.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("name = ?", name).Delete(&store.Resource{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("resource %q: %w", name, ErrNotFound)
		}
		return tx.Where("resource = ?", name).Delete(&store.Operation{}).Error
	})
}

// ListResources returns all registered resources.
func ListResources(shared *gorm.DB) ([]store.Resource, error) {
	var resources []store.Resource
	return resources, shared.Order("name").Find(&resources).Error
}

// CreateOperation registers an operation named "resource:action", creating
// the parent resource when missing.
func CreateOperation(shared *gorm.DB, name string) error {
	resource := model.OperationResource(name)
	if resource == "" {
		return fmt.Errorf("%w: operation name must have the form \"resource:action\"", ErrValidation)
	}
	return shared.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&store.Resource{Name: resource}).Error; err != nil {
			return err
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&store.Operation{Name: name, Resource: resource})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("operation %q: %w", name, ErrConflict)
		}
		return nil
	})
}

// EnsureOperation registers an operation if missing (idempotent variant used
// by bootstrap synchronization).
func EnsureOperation(shared *gorm.DB, name string) error {
	err := CreateOperation(shared, name)
	if errors.Is(err, ErrConflict) {
		return nil
	}
	return err
}

// DeleteOperation removes an operation.
func DeleteOperation(shared *gorm.DB, name string) error {
	res := shared.Where("name = ?", name).Delete(&store.Operation{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("operation %q: %w", name, ErrNotFound)
	}
	return nil
}

// ListOperations returns all registered operations.
func ListOperations(shared *gorm.DB) ([]store.Operation, error) {
	var operations []store.Operation
	return operations, shared.Order("name").Find(&operations).Error
}

func operationExists(shared *gorm.DB, name string) (bool, error) {
	var count int64
	err := shared.Model(&store.Operation{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

// --- Tenant database: roles, assignments, permissions, restricted IDs ---

// CreateRole creates a role in the tenant.
func CreateRole(tenant *gorm.DB, name string) error {
	if name == "" {
		return fmt.Errorf("%w: role name is required", ErrValidation)
	}
	res := tenant.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&store.Role{Name: name})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("role %q: %w", name, ErrConflict)
	}
	return nil
}

// EnsureRole creates a role if missing and returns it.
func EnsureRole(tenant *gorm.DB, name string) (*store.Role, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: role name is required", ErrValidation)
	}
	role := &store.Role{}
	err := tenant.Where(&store.Role{Name: name}).FirstOrCreate(role).Error
	return role, err
}

// GetRole returns a role by name.
func GetRole(tenant *gorm.DB, name string) (*store.Role, error) {
	role := &store.Role{}
	err := tenant.Where("name = ?", name).First(role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("role %q: %w", name, ErrNotFound)
	}
	return role, err
}

// RenameRole changes a role's name.
func RenameRole(tenant *gorm.DB, name, newName string) error {
	if newName == "" {
		return fmt.Errorf("%w: new role name is required", ErrValidation)
	}
	role, err := GetRole(tenant, name)
	if err != nil {
		return err
	}
	if _, err := GetRole(tenant, newName); err == nil {
		return fmt.Errorf("role %q: %w", newName, ErrConflict)
	} else if !errors.Is(err, ErrNotFound) {
		return err
	}
	return tenant.Model(role).Update("name", newName).Error
}

// DeleteRole removes a role with all its permissions, restricted IDs and
// user assignments.
func DeleteRole(tenant *gorm.DB, name string) error {
	role, err := GetRole(tenant, name)
	if err != nil {
		return err
	}
	return tenant.Transaction(func(tx *gorm.DB) error {
		for _, m := range []any{&store.UserRole{}, &store.RolePermission{}, &store.RoleRestrictedID{}} {
			if err := tx.Where("role_id = ?", role.ID).Delete(m).Error; err != nil {
				return err
			}
		}
		return tx.Delete(role).Error
	})
}

// ListRoles returns all roles in the tenant.
func ListRoles(tenant *gorm.DB) ([]store.Role, error) {
	var roles []store.Role
	return roles, tenant.Order("name").Find(&roles).Error
}

// RolePermissions returns the (operation, scope) pairs assigned to a role.
func RolePermissions(tenant *gorm.DB, name string) ([]store.RolePermission, error) {
	role, err := GetRole(tenant, name)
	if err != nil {
		return nil, err
	}
	var perms []store.RolePermission
	return perms, tenant.Where("role_id = ?", role.ID).Order("operation").Find(&perms).Error
}

// AssignRole assigns a role to a user (idempotent) and lazily provisions the
// user in the shared database.
func AssignRole(shared, tenant *gorm.DB, sub, roleName string) error {
	role, err := GetRole(tenant, roleName)
	if err != nil {
		return err
	}
	if err := EnsureUser(shared, sub); err != nil {
		return err
	}
	return tenant.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&store.UserRole{UserSub: sub, RoleID: role.ID}).Error
}

// UnassignRole removes a role from a user.
func UnassignRole(tenant *gorm.DB, sub, roleName string) error {
	role, err := GetRole(tenant, roleName)
	if err != nil {
		return err
	}
	res := tenant.Where("user_sub = ? AND role_id = ?", sub, role.ID).Delete(&store.UserRole{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("user %q has no role %q: %w", sub, roleName, ErrNotFound)
	}
	return nil
}

// UserRoles returns the names of the roles assigned to a user.
func UserRoles(tenant *gorm.DB, sub string) ([]string, error) {
	var assignments []store.UserRole
	if err := tenant.Where("user_sub = ?", sub).Find(&assignments).Error; err != nil {
		return nil, err
	}
	names := []string{}
	for _, a := range assignments {
		var role store.Role
		if err := tenant.First(&role, a.RoleID).Error; err == nil {
			names = append(names, role.Name)
		}
	}
	sort.Strings(names)
	return names, nil
}

// SetRolePermission upserts (operation, scope) on a role. The operation must
// be registered in the shared database.
func SetRolePermission(shared, tenant *gorm.DB, roleName, operation string, scope model.Scope) error {
	if !scope.Valid() {
		return fmt.Errorf("%w: invalid scope %q", ErrValidation, scope)
	}
	if ok, err := operationExists(shared, operation); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("operation %q: %w", operation, ErrNotFound)
	}
	role, err := GetRole(tenant, roleName)
	if err != nil {
		return err
	}
	return tenant.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "role_id"}, {Name: "operation"}},
		DoUpdates: clause.AssignmentColumns([]string{"scope"}),
	}).Create(&store.RolePermission{RoleID: role.ID, Operation: operation, Scope: string(scope)}).Error
}

// RemoveRolePermission deletes (operation, scope) from a role together with
// the operation's restricted IDs.
func RemoveRolePermission(tenant *gorm.DB, roleName, operation string) error {
	role, err := GetRole(tenant, roleName)
	if err != nil {
		return err
	}
	return tenant.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("role_id = ? AND operation = ?", role.ID, operation).
			Delete(&store.RolePermission{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("role %q has no permission on %q: %w", roleName, operation, ErrNotFound)
		}
		return tx.Where("role_id = ? AND operation = ?", role.ID, operation).
			Delete(&store.RoleRestrictedID{}).Error
	})
}

// SetUserPermission upserts a user-level (operation, scope) override and
// lazily provisions the user.
func SetUserPermission(shared, tenant *gorm.DB, sub, operation string, scope model.Scope) error {
	if !scope.Valid() {
		return fmt.Errorf("%w: invalid scope %q", ErrValidation, scope)
	}
	if ok, err := operationExists(shared, operation); err != nil {
		return err
	} else if !ok {
		return fmt.Errorf("operation %q: %w", operation, ErrNotFound)
	}
	if err := EnsureUser(shared, sub); err != nil {
		return err
	}
	return tenant.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_sub"}, {Name: "operation"}},
		DoUpdates: clause.AssignmentColumns([]string{"scope"}),
	}).Create(&store.UserPermission{UserSub: sub, Operation: operation, Scope: string(scope)}).Error
}

// RemoveUserPermission deletes a user-level override together with the
// operation's restricted IDs.
func RemoveUserPermission(tenant *gorm.DB, sub, operation string) error {
	return tenant.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("user_sub = ? AND operation = ?", sub, operation).
			Delete(&store.UserPermission{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("user %q has no permission on %q: %w", sub, operation, ErrNotFound)
		}
		return tx.Where("user_sub = ? AND operation = ?", sub, operation).
			Delete(&store.UserRestrictedID{}).Error
	})
}

// UpdateRoleRestrictedIDs adds and removes record IDs for a role's
// RESTRICTED permission on an operation.
func UpdateRoleRestrictedIDs(tenant *gorm.DB, roleName, operation string, add, remove []string) error {
	role, err := GetRole(tenant, roleName)
	if err != nil {
		return err
	}
	var count int64
	if err := tenant.Model(&store.RolePermission{}).
		Where("role_id = ? AND operation = ?", role.ID, operation).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("role %q has no permission on %q: %w", roleName, operation, ErrNotFound)
	}
	return tenant.Transaction(func(tx *gorm.DB) error {
		for _, id := range add {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
				Create(&store.RoleRestrictedID{RoleID: role.ID, Operation: operation, RecordID: id}).Error; err != nil {
				return err
			}
		}
		if len(remove) > 0 {
			if err := tx.Where("role_id = ? AND operation = ? AND record_id IN ?", role.ID, operation, remove).
				Delete(&store.RoleRestrictedID{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

// UpdateUserRestrictedIDs adds and removes record IDs for a user's
// RESTRICTED permission on an operation.
func UpdateUserRestrictedIDs(tenant *gorm.DB, sub, operation string, add, remove []string) error {
	var count int64
	if err := tenant.Model(&store.UserPermission{}).
		Where("user_sub = ? AND operation = ?", sub, operation).Count(&count).Error; err != nil {
		return err
	}
	if count == 0 {
		return fmt.Errorf("user %q has no permission on %q: %w", sub, operation, ErrNotFound)
	}
	return tenant.Transaction(func(tx *gorm.DB) error {
		for _, id := range add {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
				Create(&store.UserRestrictedID{UserSub: sub, Operation: operation, RecordID: id}).Error; err != nil {
				return err
			}
		}
		if len(remove) > 0 {
			if err := tx.Where("user_sub = ? AND operation = ? AND record_id IN ?", sub, operation, remove).
				Delete(&store.UserRestrictedID{}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}
