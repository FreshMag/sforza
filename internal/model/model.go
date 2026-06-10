// Package model defines the core SFBAC domain types shared across layers.
package model

import "strings"

// Scope is the visibility scope attached to an (operation, subject) pair.
type Scope string

const (
	// ScopeFull grants access to all records of the operation.
	ScopeFull Scope = "FULL"
	// ScopeEmpty grants access to no records.
	ScopeEmpty Scope = "EMPTY"
	// ScopeRestricted grants access to an explicit set of record IDs.
	ScopeRestricted Scope = "RESTRICTED"
)

// Valid reports whether s is one of the supported scopes.
func (s Scope) Valid() bool {
	switch s {
	case ScopeFull, ScopeEmpty, ScopeRestricted:
		return true
	}
	return false
}

// OperationScope is the effective (operation, scope) pair returned by
// permission queries.
type OperationScope struct {
	Operation string `json:"operation"`
	Scope     Scope  `json:"scope"`
}

// OperationResource extracts the resource part of an operation name
// ("product:read" -> "product"). It returns "" when the name has no
// "resource:action" shape.
func OperationResource(operation string) string {
	res, _, ok := strings.Cut(operation, ":")
	if !ok || res == "" {
		return ""
	}
	return res
}

// AdminRole is the bootstrap administrator role created in every tenant.
const AdminRole = "authorization:admin"

// Meta operation names used to authorize Sforza's own administrative APIs.
const (
	MetaRoleRead        = "role:read"
	MetaRoleWrite       = "role:write"
	MetaRoleAssign      = "role:assign"
	MetaOperationRead   = "operation:read"
	MetaOperationWrite  = "operation:write"
	MetaOperationAssign = "operation:assign"
	MetaResourceRead    = "resource:read"
	MetaResourceWrite   = "resource:write"
	MetaUserRead        = "user:read"
)

// MetaOperations maps every meta operation to its meta resource.
var MetaOperations = map[string]string{
	MetaRoleRead:        "role",
	MetaRoleWrite:       "role",
	MetaRoleAssign:      "role",
	MetaOperationRead:   "operation",
	MetaOperationWrite:  "operation",
	MetaOperationAssign: "operation",
	MetaResourceRead:    "resource",
	MetaResourceWrite:   "resource",
	MetaUserRead:        "user",
}

// IsMetaOperation reports whether the operation belongs to Sforza's own
// meta authorization model.
func IsMetaOperation(operation string) bool {
	_, ok := MetaOperations[operation]
	return ok
}
