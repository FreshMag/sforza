package store

// Shared database models. Resources, operations and users are global.

// Resource is a logical domain entity that groups operations.
type Resource struct {
	ID   uint   `gorm:"primaryKey" json:"-"`
	Name string `gorm:"uniqueIndex;not null" json:"name"`
}

// Operation is an action on a resource, named "resource:action".
type Operation struct {
	ID       uint   `gorm:"primaryKey" json:"-"`
	Name     string `gorm:"uniqueIndex;not null" json:"name"`
	Resource string `gorm:"index;not null" json:"resource"`
}

// User is a lazily provisioned principal identified by its OIDC sub claim.
type User struct {
	ID  uint   `gorm:"primaryKey" json:"-"`
	Sub string `gorm:"uniqueIndex;not null" json:"sub"`
}

// Tenant database models. Subjects and operations are referenced by value
// (sub / operation name) because they live in the shared database.

// Role is a named set of (operation, scope) assignments within a tenant.
type Role struct {
	ID   uint   `gorm:"primaryKey" json:"-"`
	Name string `gorm:"uniqueIndex;not null" json:"name"`
}

// UserRole assigns a role to a user within a tenant.
type UserRole struct {
	ID      uint   `gorm:"primaryKey"`
	UserSub string `gorm:"uniqueIndex:idx_user_role;not null"`
	RoleID  uint   `gorm:"uniqueIndex:idx_user_role;not null"`
}

// RolePermission binds (operation, scope) to a role.
type RolePermission struct {
	ID        uint   `gorm:"primaryKey" json:"-"`
	RoleID    uint   `gorm:"uniqueIndex:idx_role_perm;not null" json:"-"`
	Operation string `gorm:"uniqueIndex:idx_role_perm;not null" json:"operation"`
	Scope     string `gorm:"not null" json:"scope"`
}

// UserPermission binds (operation, scope) directly to a user; it always
// overrides role permissions for the same operation.
type UserPermission struct {
	ID        uint   `gorm:"primaryKey" json:"-"`
	UserSub   string `gorm:"uniqueIndex:idx_user_perm;not null" json:"-"`
	Operation string `gorm:"uniqueIndex:idx_user_perm;not null" json:"operation"`
	Scope     string `gorm:"not null" json:"scope"`
}

// RoleRestrictedID is one record ID accessible through a role's RESTRICTED
// permission on an operation.
type RoleRestrictedID struct {
	ID        uint   `gorm:"primaryKey"`
	RoleID    uint   `gorm:"uniqueIndex:idx_role_rid;not null"`
	Operation string `gorm:"uniqueIndex:idx_role_rid;not null"`
	RecordID  string `gorm:"uniqueIndex:idx_role_rid;not null"`
}

// UserRestrictedID is one record ID accessible through a user's RESTRICTED
// permission on an operation.
type UserRestrictedID struct {
	ID        uint   `gorm:"primaryKey"`
	UserSub   string `gorm:"uniqueIndex:idx_user_rid;not null"`
	Operation string `gorm:"uniqueIndex:idx_user_rid;not null"`
	RecordID  string `gorm:"uniqueIndex:idx_user_rid;not null"`
}

func sharedModels() []any {
	return []any{&Resource{}, &Operation{}, &User{}}
}

func tenantModels() []any {
	return []any{
		&Role{}, &UserRole{}, &RolePermission{}, &UserPermission{},
		&RoleRestrictedID{}, &UserRestrictedID{},
	}
}
