package service

import (
	"fmt"
	"sort"

	"gorm.io/gorm"

	"github.com/francesco/sforza/internal/model"
	"github.com/francesco/sforza/internal/store"
)

// resolution describes how an operation's effective scope was derived.
type resolution struct {
	scope    model.Scope
	fromUser bool   // true when a user override produced the scope
	roleIDs  []uint // roles contributing RESTRICTED ids (when !fromUser)
}

// resolve computes the effective permission set of a user inside a tenant.
//
// Rules:
//   - A user-level permission always overrides role permissions.
//   - Across multiple roles the widest scope wins: FULL > RESTRICTED > EMPTY.
//   - When several roles grant RESTRICTED, their ID sets are unioned.
//   - Operations with no assignment at all are absent (deny-by-default).
func resolve(tenant *gorm.DB, sub string) (map[string]*resolution, error) {
	eff := map[string]*resolution{}

	var userPerms []store.UserPermission
	if err := tenant.Where("user_sub = ?", sub).Find(&userPerms).Error; err != nil {
		return nil, fmt.Errorf("load user permissions: %w", err)
	}
	for _, p := range userPerms {
		eff[p.Operation] = &resolution{scope: model.Scope(p.Scope), fromUser: true}
	}

	var assignments []store.UserRole
	if err := tenant.Where("user_sub = ?", sub).Find(&assignments).Error; err != nil {
		return nil, fmt.Errorf("load role assignments: %w", err)
	}
	if len(assignments) == 0 {
		return eff, nil
	}
	roleIDs := make([]uint, 0, len(assignments))
	for _, a := range assignments {
		roleIDs = append(roleIDs, a.RoleID)
	}

	var rolePerms []store.RolePermission
	if err := tenant.Where("role_id IN ?", roleIDs).Find(&rolePerms).Error; err != nil {
		return nil, fmt.Errorf("load role permissions: %w", err)
	}
	for _, p := range rolePerms {
		cur, ok := eff[p.Operation]
		if ok && cur.fromUser {
			continue // user override wins
		}
		scope := model.Scope(p.Scope)
		switch {
		case !ok:
			r := &resolution{scope: scope}
			if scope == model.ScopeRestricted {
				r.roleIDs = []uint{p.RoleID}
			}
			eff[p.Operation] = r
		case cur.scope == model.ScopeFull:
			// FULL already; nothing can widen it.
		case scope == model.ScopeFull:
			cur.scope = model.ScopeFull
			cur.roleIDs = nil
		case scope == model.ScopeRestricted:
			cur.scope = model.ScopeRestricted
			cur.roleIDs = append(cur.roleIDs, p.RoleID)
		}
	}
	return eff, nil
}

// EffectiveOperations returns every operation available to the user with its
// effective scope, sorted by operation name. Restricted IDs are never
// included here.
func EffectiveOperations(tenant *gorm.DB, sub string) ([]model.OperationScope, error) {
	eff, err := resolve(tenant, sub)
	if err != nil {
		return nil, err
	}
	out := make([]model.OperationScope, 0, len(eff))
	for op, r := range eff {
		out = append(out, model.OperationScope{Operation: op, Scope: r.scope})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Operation < out[j].Operation })
	return out, nil
}

// MetaOperations returns the subset of effective operations that belong to
// Sforza's own meta authorization model.
func MetaOperations(tenant *gorm.DB, sub string) ([]model.OperationScope, error) {
	all, err := EffectiveOperations(tenant, sub)
	if err != nil {
		return nil, err
	}
	out := make([]model.OperationScope, 0, len(all))
	for _, os := range all {
		if model.IsMetaOperation(os.Operation) {
			out = append(out, os)
		}
	}
	return out, nil
}

// RecordIDs returns, for each requested operation whose effective scope is
// RESTRICTED, the sorted set of accessible record IDs. Operations with FULL,
// EMPTY or no assignment are omitted from the result.
func RecordIDs(tenant *gorm.DB, sub string, operations []string) (map[string][]string, error) {
	eff, err := resolve(tenant, sub)
	if err != nil {
		return nil, err
	}
	out := map[string][]string{}
	for _, op := range operations {
		r, ok := eff[op]
		if !ok || r.scope != model.ScopeRestricted {
			continue
		}
		var ids []string
		if r.fromUser {
			var rows []store.UserRestrictedID
			if err := tenant.Where("user_sub = ? AND operation = ?", sub, op).Find(&rows).Error; err != nil {
				return nil, fmt.Errorf("load user restricted ids: %w", err)
			}
			for _, row := range rows {
				ids = append(ids, row.RecordID)
			}
		} else {
			var rows []store.RoleRestrictedID
			if err := tenant.Where("role_id IN ? AND operation = ?", r.roleIDs, op).Find(&rows).Error; err != nil {
				return nil, fmt.Errorf("load role restricted ids: %w", err)
			}
			seen := map[string]bool{}
			for _, row := range rows {
				if !seen[row.RecordID] {
					seen[row.RecordID] = true
					ids = append(ids, row.RecordID)
				}
			}
		}
		if ids == nil {
			ids = []string{}
		}
		sort.Strings(ids)
		out[op] = ids
	}
	return out, nil
}

// Authorize reports whether the user holds the operation with FULL scope.
// Sforza's administrative APIs operate on whole collections, so meta
// permissions are only honored at FULL scope; RESTRICTED and EMPTY deny.
func Authorize(tenant *gorm.DB, sub, operation string) (bool, error) {
	eff, err := resolve(tenant, sub)
	if err != nil {
		return false, err
	}
	r, ok := eff[operation]
	return ok && r.scope == model.ScopeFull, nil
}
