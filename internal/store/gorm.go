package store

import (
	"errors"
	"fmt"
	"strings"

	"github.com/glebarez/sqlite"
	"gorm.io/driver/mysql"
	"gorm.io/driver/postgres"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
	"gorm.io/gorm/logger"

	"github.com/FreshMag/sforza/internal/config"
	"github.com/FreshMag/sforza/internal/model"
)

// Indexed string columns carry size:191 so unique indexes fit MySQL's
// utf8mb4 index length limit; other dialects ignore the size.

type userModel struct {
	ID  uint   `gorm:"primaryKey"`
	Sub string `gorm:"uniqueIndex;size:191;not null"`
}

func (userModel) TableName() string { return "users" }

type resourceModel struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"uniqueIndex;size:191;not null"`
}

func (resourceModel) TableName() string { return "resources" }

type operationModel struct {
	ID       uint   `gorm:"primaryKey"`
	Name     string `gorm:"uniqueIndex;size:191;not null"`
	Resource string `gorm:"index;size:191;not null"`
}

func (operationModel) TableName() string { return "operations" }

type roleModel struct {
	ID   uint   `gorm:"primaryKey"`
	Name string `gorm:"uniqueIndex;size:191;not null"`
}

func (roleModel) TableName() string { return "roles" }

type userRoleModel struct {
	ID      uint   `gorm:"primaryKey"`
	UserSub string `gorm:"uniqueIndex:idx_user_role;size:191;not null"`
	RoleID  uint   `gorm:"uniqueIndex:idx_user_role;not null"`
}

func (userRoleModel) TableName() string { return "user_roles" }

type rolePermissionModel struct {
	ID        uint   `gorm:"primaryKey"`
	RoleID    uint   `gorm:"uniqueIndex:idx_role_perm;not null"`
	Operation string `gorm:"uniqueIndex:idx_role_perm;size:191;not null"`
	Scope     string `gorm:"not null"`
}

func (rolePermissionModel) TableName() string { return "role_permissions" }

type userPermissionModel struct {
	ID        uint   `gorm:"primaryKey"`
	UserSub   string `gorm:"uniqueIndex:idx_user_perm;size:191;not null"`
	Operation string `gorm:"uniqueIndex:idx_user_perm;size:191;not null"`
	Scope     string `gorm:"not null"`
}

func (userPermissionModel) TableName() string { return "user_permissions" }

type roleRestrictedIDModel struct {
	ID        uint   `gorm:"primaryKey"`
	RoleID    uint   `gorm:"uniqueIndex:idx_role_rid;not null"`
	Operation string `gorm:"uniqueIndex:idx_role_rid;size:191;not null"`
	RecordID  string `gorm:"uniqueIndex:idx_role_rid;size:191;not null"`
}

func (roleRestrictedIDModel) TableName() string { return "role_restricted_ids" }

type userRestrictedIDModel struct {
	ID        uint   `gorm:"primaryKey"`
	UserSub   string `gorm:"uniqueIndex:idx_user_rid;size:191;not null"`
	Operation string `gorm:"uniqueIndex:idx_user_rid;size:191;not null"`
	RecordID  string `gorm:"uniqueIndex:idx_user_rid;size:191;not null"`
}

func (userRestrictedIDModel) TableName() string { return "user_restricted_ids" }

func sharedModels() []any {
	return []any{&resourceModel{}, &operationModel{}, &userModel{}}
}

func tenantModels() []any {
	return []any{
		&roleModel{}, &userRoleModel{}, &rolePermissionModel{}, &userPermissionModel{},
		&roleRestrictedIDModel{}, &userRestrictedIDModel{},
	}
}

func openGorm(cfg config.DB) (*gorm.DB, error) {
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
	case "mysql":
		return gorm.Open(mysql.Open(cfg.DSN), gormCfg)
	default:
		return nil, fmt.Errorf("unsupported gorm driver %q", cfg.Driver)
	}
}

// --- Shared implementation ---

type gormShared struct {
	db *gorm.DB
}

func (s *gormShared) EnsureUser(sub string) error {
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&userModel{Sub: sub}).Error
}

func (s *gormShared) ListUsers() ([]User, error) {
	var rows []userModel
	if err := s.db.Order("sub").Find(&rows).Error; err != nil {
		return nil, err
	}
	users := make([]User, 0, len(rows))
	for _, r := range rows {
		users = append(users, User{Sub: r.Sub})
	}
	return users, nil
}

func (s *gormShared) CreateResource(name string) error {
	res := s.db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&resourceModel{Name: name})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("resource %q: %w", name, ErrConflict)
	}
	return nil
}

func (s *gormShared) EnsureResource(name string) error {
	return s.db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&resourceModel{Name: name}).Error
}

func (s *gormShared) DeleteResource(name string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("name = ?", name).Delete(&resourceModel{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("resource %q: %w", name, ErrNotFound)
		}
		return tx.Where("resource = ?", name).Delete(&operationModel{}).Error
	})
}

func (s *gormShared) ListResources() ([]Resource, error) {
	var rows []resourceModel
	if err := s.db.Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	resources := make([]Resource, 0, len(rows))
	for _, r := range rows {
		resources = append(resources, Resource{Name: r.Name})
	}
	return resources, nil
}

func (s *gormShared) CreateOperation(name, resource string) error {
	return s.db.Transaction(func(tx *gorm.DB) error {
		if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&resourceModel{Name: resource}).Error; err != nil {
			return err
		}
		res := tx.Clauses(clause.OnConflict{DoNothing: true}).
			Create(&operationModel{Name: name, Resource: resource})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("operation %q: %w", name, ErrConflict)
		}
		return nil
	})
}

func (s *gormShared) EnsureOperation(name, resource string) error {
	err := s.CreateOperation(name, resource)
	if errors.Is(err, ErrConflict) {
		return nil
	}
	return err
}

func (s *gormShared) DeleteOperation(name string) error {
	res := s.db.Where("name = ?", name).Delete(&operationModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("operation %q: %w", name, ErrNotFound)
	}
	return nil
}

func (s *gormShared) ListOperations() ([]Operation, error) {
	var rows []operationModel
	if err := s.db.Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	operations := make([]Operation, 0, len(rows))
	for _, r := range rows {
		operations = append(operations, Operation{Name: r.Name, Resource: r.Resource})
	}
	return operations, nil
}

func (s *gormShared) OperationExists(name string) (bool, error) {
	var count int64
	err := s.db.Model(&operationModel{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

// --- Tenant implementation ---

type gormTenant struct {
	db *gorm.DB
}

func (t *gormTenant) roleByName(name string) (*roleModel, error) {
	role := &roleModel{}
	err := t.db.Where("name = ?", name).First(role).Error
	if errors.Is(err, gorm.ErrRecordNotFound) {
		return nil, fmt.Errorf("role %q: %w", name, ErrNotFound)
	}
	return role, err
}

func (t *gormTenant) CreateRole(name string) error {
	res := t.db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&roleModel{Name: name})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("role %q: %w", name, ErrConflict)
	}
	return nil
}

func (t *gormTenant) EnsureRole(name string) error {
	return t.db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&roleModel{Name: name}).Error
}

func (t *gormTenant) RoleExists(name string) (bool, error) {
	var count int64
	err := t.db.Model(&roleModel{}).Where("name = ?", name).Count(&count).Error
	return count > 0, err
}

func (t *gormTenant) RenameRole(name, newName string) error {
	role, err := t.roleByName(name)
	if err != nil {
		return err
	}
	if exists, err := t.RoleExists(newName); err != nil {
		return err
	} else if exists {
		return fmt.Errorf("role %q: %w", newName, ErrConflict)
	}
	return t.db.Model(role).Update("name", newName).Error
}

func (t *gormTenant) DeleteRole(name string) error {
	role, err := t.roleByName(name)
	if err != nil {
		return err
	}
	return t.db.Transaction(func(tx *gorm.DB) error {
		for _, m := range []any{&userRoleModel{}, &rolePermissionModel{}, &roleRestrictedIDModel{}} {
			if err := tx.Where("role_id = ?", role.ID).Delete(m).Error; err != nil {
				return err
			}
		}
		return tx.Delete(role).Error
	})
}

func (t *gormTenant) ListRoles() ([]Role, error) {
	var rows []roleModel
	if err := t.db.Order("name").Find(&rows).Error; err != nil {
		return nil, err
	}
	roles := make([]Role, 0, len(rows))
	for _, r := range rows {
		roles = append(roles, Role{Name: r.Name})
	}
	return roles, nil
}

func (t *gormTenant) AssignRole(sub, role string) error {
	r, err := t.roleByName(role)
	if err != nil {
		return err
	}
	return t.db.Clauses(clause.OnConflict{DoNothing: true}).
		Create(&userRoleModel{UserSub: sub, RoleID: r.ID}).Error
}

func (t *gormTenant) UnassignRole(sub, role string) error {
	r, err := t.roleByName(role)
	if err != nil {
		return err
	}
	res := t.db.Where("user_sub = ? AND role_id = ?", sub, r.ID).Delete(&userRoleModel{})
	if res.Error != nil {
		return res.Error
	}
	if res.RowsAffected == 0 {
		return fmt.Errorf("user %q has no role %q: %w", sub, role, ErrNotFound)
	}
	return nil
}

func (t *gormTenant) UserRoles(sub string) ([]string, error) {
	var assignments []userRoleModel
	if err := t.db.Where("user_sub = ?", sub).Find(&assignments).Error; err != nil {
		return nil, err
	}
	if len(assignments) == 0 {
		return []string{}, nil
	}
	ids := make([]uint, 0, len(assignments))
	for _, a := range assignments {
		ids = append(ids, a.RoleID)
	}
	var roles []roleModel
	if err := t.db.Where("id IN ?", ids).Order("name").Find(&roles).Error; err != nil {
		return nil, err
	}
	names := make([]string, 0, len(roles))
	for _, r := range roles {
		names = append(names, r.Name)
	}
	return names, nil
}

func (t *gormTenant) SetRolePermission(role, operation string, scope model.Scope) error {
	r, err := t.roleByName(role)
	if err != nil {
		return err
	}
	return t.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "role_id"}, {Name: "operation"}},
		DoUpdates: clause.AssignmentColumns([]string{"scope"}),
	}).Create(&rolePermissionModel{RoleID: r.ID, Operation: operation, Scope: string(scope)}).Error
}

func (t *gormTenant) RemoveRolePermission(role, operation string) error {
	r, err := t.roleByName(role)
	if err != nil {
		return err
	}
	return t.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("role_id = ? AND operation = ?", r.ID, operation).
			Delete(&rolePermissionModel{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("role %q has no permission on %q: %w", role, operation, ErrNotFound)
		}
		return tx.Where("role_id = ? AND operation = ?", r.ID, operation).
			Delete(&roleRestrictedIDModel{}).Error
	})
}

func (t *gormTenant) RolePermissions(role string) ([]model.OperationScope, error) {
	r, err := t.roleByName(role)
	if err != nil {
		return nil, err
	}
	var rows []rolePermissionModel
	if err := t.db.Where("role_id = ?", r.ID).Order("operation").Find(&rows).Error; err != nil {
		return nil, err
	}
	perms := make([]model.OperationScope, 0, len(rows))
	for _, p := range rows {
		perms = append(perms, model.OperationScope{Operation: p.Operation, Scope: model.Scope(p.Scope)})
	}
	return perms, nil
}

func (t *gormTenant) HasRolePermission(role, operation string) (bool, error) {
	r, err := t.roleByName(role)
	if err != nil {
		return false, err
	}
	var count int64
	err = t.db.Model(&rolePermissionModel{}).
		Where("role_id = ? AND operation = ?", r.ID, operation).Count(&count).Error
	return count > 0, err
}

func (t *gormTenant) SetUserPermission(sub, operation string, scope model.Scope) error {
	return t.db.Clauses(clause.OnConflict{
		Columns:   []clause.Column{{Name: "user_sub"}, {Name: "operation"}},
		DoUpdates: clause.AssignmentColumns([]string{"scope"}),
	}).Create(&userPermissionModel{UserSub: sub, Operation: operation, Scope: string(scope)}).Error
}

func (t *gormTenant) RemoveUserPermission(sub, operation string) error {
	return t.db.Transaction(func(tx *gorm.DB) error {
		res := tx.Where("user_sub = ? AND operation = ?", sub, operation).
			Delete(&userPermissionModel{})
		if res.Error != nil {
			return res.Error
		}
		if res.RowsAffected == 0 {
			return fmt.Errorf("user %q has no permission on %q: %w", sub, operation, ErrNotFound)
		}
		return tx.Where("user_sub = ? AND operation = ?", sub, operation).
			Delete(&userRestrictedIDModel{}).Error
	})
}

func (t *gormTenant) UserPermissions(sub string) ([]model.OperationScope, error) {
	var rows []userPermissionModel
	if err := t.db.Where("user_sub = ?", sub).Order("operation").Find(&rows).Error; err != nil {
		return nil, err
	}
	perms := make([]model.OperationScope, 0, len(rows))
	for _, p := range rows {
		perms = append(perms, model.OperationScope{Operation: p.Operation, Scope: model.Scope(p.Scope)})
	}
	return perms, nil
}

func (t *gormTenant) HasUserPermission(sub, operation string) (bool, error) {
	var count int64
	err := t.db.Model(&userPermissionModel{}).
		Where("user_sub = ? AND operation = ?", sub, operation).Count(&count).Error
	return count > 0, err
}

func (t *gormTenant) AddRoleRestrictedIDs(role, operation string, ids []string) error {
	r, err := t.roleByName(role)
	if err != nil {
		return err
	}
	return t.db.Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
				Create(&roleRestrictedIDModel{RoleID: r.ID, Operation: operation, RecordID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (t *gormTenant) RemoveRoleRestrictedIDs(role, operation string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	r, err := t.roleByName(role)
	if err != nil {
		return err
	}
	return t.db.Where("role_id = ? AND operation = ? AND record_id IN ?", r.ID, operation, ids).
		Delete(&roleRestrictedIDModel{}).Error
}

func (t *gormTenant) RoleRestrictedIDs(roles []string, operation string) ([]string, error) {
	if len(roles) == 0 {
		return []string{}, nil
	}
	var roleRows []roleModel
	if err := t.db.Where("name IN ?", roles).Find(&roleRows).Error; err != nil {
		return nil, err
	}
	ids := make([]uint, 0, len(roleRows))
	for _, r := range roleRows {
		ids = append(ids, r.ID)
	}
	var rows []roleRestrictedIDModel
	if err := t.db.Where("role_id IN ? AND operation = ?", ids, operation).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []string{}
	seen := map[string]bool{}
	for _, row := range rows {
		if !seen[row.RecordID] {
			seen[row.RecordID] = true
			out = append(out, row.RecordID)
		}
	}
	return out, nil
}

func (t *gormTenant) AddUserRestrictedIDs(sub, operation string, ids []string) error {
	return t.db.Transaction(func(tx *gorm.DB) error {
		for _, id := range ids {
			if err := tx.Clauses(clause.OnConflict{DoNothing: true}).
				Create(&userRestrictedIDModel{UserSub: sub, Operation: operation, RecordID: id}).Error; err != nil {
				return err
			}
		}
		return nil
	})
}

func (t *gormTenant) RemoveUserRestrictedIDs(sub, operation string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	return t.db.Where("user_sub = ? AND operation = ? AND record_id IN ?", sub, operation, ids).
		Delete(&userRestrictedIDModel{}).Error
}

func (t *gormTenant) UserRestrictedIDs(sub, operation string) ([]string, error) {
	var rows []userRestrictedIDModel
	if err := t.db.Where("user_sub = ? AND operation = ?", sub, operation).Find(&rows).Error; err != nil {
		return nil, err
	}
	out := []string{}
	for _, row := range rows {
		out = append(out, row.RecordID)
	}
	return out, nil
}

func (t *gormTenant) RoleGrants(roles []string) ([]RoleGrant, error) {
	if len(roles) == 0 {
		return []RoleGrant{}, nil
	}
	var roleRows []roleModel
	if err := t.db.Where("name IN ?", roles).Find(&roleRows).Error; err != nil {
		return nil, err
	}
	names := map[uint]string{}
	ids := make([]uint, 0, len(roleRows))
	for _, r := range roleRows {
		names[r.ID] = r.Name
		ids = append(ids, r.ID)
	}
	var rows []rolePermissionModel
	if err := t.db.Where("role_id IN ?", ids).Find(&rows).Error; err != nil {
		return nil, err
	}
	grants := make([]RoleGrant, 0, len(rows))
	for _, p := range rows {
		grants = append(grants, RoleGrant{
			Role:      names[p.RoleID],
			Operation: p.Operation,
			Scope:     model.Scope(p.Scope),
		})
	}
	return grants, nil
}
