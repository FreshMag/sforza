package service

import (
	"fmt"
	"sort"

	"github.com/FreshMag/sforza/internal/model"
	"github.com/FreshMag/sforza/internal/store"
)

// resolution describes how an operation's effective scope was derived.
type resolution struct {
	scope    model.Scope
	fromUser bool     // true when a user override produced the scope
	roles    []string // roles contributing RESTRICTED ids (when !fromUser)
}

// resolve computes the effective permission set of a user inside a tenant.
//
// Rules:
//   - A user-level permission always overrides role permissions.
//   - Across multiple roles the widest scope wins: FULL > RESTRICTED > EMPTY.
//   - When several roles grant RESTRICTED, their ID sets are unioned.
//   - Operations with no assignment at all are absent (deny-by-default).
func resolve(tenant store.Tenant, sub string) (map[string]*resolution, error) {
	eff := map[string]*resolution{}

	userPerms, err := tenant.UserPermissions(sub)
	if err != nil {
		return nil, fmt.Errorf("load user permissions: %w", err)
	}
	for _, p := range userPerms {
		eff[p.Operation] = &resolution{scope: p.Scope, fromUser: true}
	}

	roles, err := tenant.UserRoles(sub)
	if err != nil {
		return nil, fmt.Errorf("load role assignments: %w", err)
	}
	if len(roles) == 0 {
		return eff, nil
	}

	grants, err := tenant.RoleGrants(roles)
	if err != nil {
		return nil, fmt.Errorf("load role permissions: %w", err)
	}
	for _, g := range grants {
		cur, ok := eff[g.Operation]
		if ok && cur.fromUser {
			continue // user override wins
		}
		switch {
		case !ok:
			r := &resolution{scope: g.Scope}
			if g.Scope == model.ScopeRestricted {
				r.roles = []string{g.Role}
			}
			eff[g.Operation] = r
		case cur.scope == model.ScopeFull:
			// FULL already; nothing can widen it.
		case g.Scope == model.ScopeFull:
			cur.scope = model.ScopeFull
			cur.roles = nil
		case g.Scope == model.ScopeRestricted:
			cur.scope = model.ScopeRestricted
			cur.roles = append(cur.roles, g.Role)
		}
	}
	return eff, nil
}

// EffectiveOperations returns every operation available to the user with its
// effective scope, sorted by operation name. Restricted IDs are never
// included here.
func EffectiveOperations(tenant store.Tenant, sub string) ([]model.OperationScope, error) {
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
func MetaOperations(tenant store.Tenant, sub string) ([]model.OperationScope, error) {
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
func RecordIDs(tenant store.Tenant, sub string, operations []string) (map[string][]string, error) {
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
			ids, err = tenant.UserRestrictedIDs(sub, op)
		} else {
			ids, err = tenant.RoleRestrictedIDs(r.roles, op)
		}
		if err != nil {
			return nil, fmt.Errorf("load restricted ids for %q: %w", op, err)
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
func Authorize(tenant store.Tenant, sub, operation string) (bool, error) {
	eff, err := resolve(tenant, sub)
	if err != nil {
		return false, err
	}
	r, ok := eff[operation]
	return ok && r.scope == model.ScopeFull, nil
}
