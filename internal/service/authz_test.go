package service_test

import (
	"reflect"
	"testing"

	"gorm.io/gorm"

	"github.com/francesco/sforza/internal/model"
	"github.com/francesco/sforza/internal/service"
	"github.com/francesco/sforza/internal/store"
	"github.com/francesco/sforza/internal/testutil"
)

func setup(t *testing.T) (*store.Stores, *gorm.DB) {
	t.Helper()
	stores := testutil.NewStores(t)
	tenant, _ := stores.Tenant("tenant-a")
	for _, op := range []string{"product:read", "product:write", "invoice:read", "invoice:approve"} {
		if err := service.EnsureOperation(stores.Shared, op); err != nil {
			t.Fatal(err)
		}
	}
	return stores, tenant
}

func mustScope(t *testing.T, ops []model.OperationScope, operation string) model.Scope {
	t.Helper()
	for _, os := range ops {
		if os.Operation == operation {
			return os.Scope
		}
	}
	t.Fatalf("operation %q not in effective set %v", operation, ops)
	return ""
}

func hasOperation(ops []model.OperationScope, operation string) bool {
	for _, os := range ops {
		if os.Operation == operation {
			return true
		}
	}
	return false
}

func TestDenyByDefault(t *testing.T) {
	_, tenant := setup(t)
	ops, err := service.EffectiveOperations(tenant, "nobody")
	if err != nil {
		t.Fatal(err)
	}
	if len(ops) != 0 {
		t.Errorf("user without assignments must have no operations, got %v", ops)
	}
	ok, err := service.Authorize(tenant, "nobody", "product:read")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("Authorize must deny by default")
	}
}

func TestRolePermissions(t *testing.T) {
	stores, tenant := setup(t)
	if err := service.CreateRole(tenant, "manager"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetRolePermission(stores.Shared, tenant, "manager", "product:read", model.ScopeFull); err != nil {
		t.Fatal(err)
	}
	if err := service.SetRolePermission(stores.Shared, tenant, "manager", "invoice:read", model.ScopeRestricted); err != nil {
		t.Fatal(err)
	}
	if err := service.AssignRole(stores.Shared, tenant, "pippo", "manager"); err != nil {
		t.Fatal(err)
	}

	ops, err := service.EffectiveOperations(tenant, "pippo")
	if err != nil {
		t.Fatal(err)
	}
	if got := mustScope(t, ops, "product:read"); got != model.ScopeFull {
		t.Errorf("product:read = %v, want FULL", got)
	}
	if got := mustScope(t, ops, "invoice:read"); got != model.ScopeRestricted {
		t.Errorf("invoice:read = %v, want RESTRICTED", got)
	}
	if hasOperation(ops, "product:write") {
		t.Error("product:write must not be granted")
	}
}

func TestUserOverrideWinsOverRole(t *testing.T) {
	stores, tenant := setup(t)
	if err := service.CreateRole(tenant, "manager"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetRolePermission(stores.Shared, tenant, "manager", "product:read", model.ScopeRestricted); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateRoleRestrictedIDs(tenant, "manager", "product:read", []string{"1", "2", "3"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := service.AssignRole(stores.Shared, tenant, "pippo", "manager"); err != nil {
		t.Fatal(err)
	}
	// Override: the user gets FULL even though the role says RESTRICTED.
	if err := service.SetUserPermission(stores.Shared, tenant, "pippo", "product:read", model.ScopeFull); err != nil {
		t.Fatal(err)
	}

	ops, err := service.EffectiveOperations(tenant, "pippo")
	if err != nil {
		t.Fatal(err)
	}
	if got := mustScope(t, ops, "product:read"); got != model.ScopeFull {
		t.Errorf("product:read = %v, want FULL (user override)", got)
	}

	// An EMPTY override must also win over a FULL role grant.
	if err := service.SetRolePermission(stores.Shared, tenant, "manager", "invoice:read", model.ScopeFull); err != nil {
		t.Fatal(err)
	}
	if err := service.SetUserPermission(stores.Shared, tenant, "pippo", "invoice:read", model.ScopeEmpty); err != nil {
		t.Fatal(err)
	}
	ops, err = service.EffectiveOperations(tenant, "pippo")
	if err != nil {
		t.Fatal(err)
	}
	if got := mustScope(t, ops, "invoice:read"); got != model.ScopeEmpty {
		t.Errorf("invoice:read = %v, want EMPTY (user override)", got)
	}
}

func TestMultipleRolesWidestScopeWins(t *testing.T) {
	stores, tenant := setup(t)
	for role, scope := range map[string]model.Scope{
		"empty-role":      model.ScopeEmpty,
		"restricted-role": model.ScopeRestricted,
		"full-role":       model.ScopeFull,
	} {
		if err := service.CreateRole(tenant, role); err != nil {
			t.Fatal(err)
		}
		if err := service.SetRolePermission(stores.Shared, tenant, role, "product:read", scope); err != nil {
			t.Fatal(err)
		}
	}

	for _, tc := range []struct {
		user  string
		roles []string
		want  model.Scope
	}{
		{"u1", []string{"empty-role"}, model.ScopeEmpty},
		{"u2", []string{"empty-role", "restricted-role"}, model.ScopeRestricted},
		{"u3", []string{"empty-role", "restricted-role", "full-role"}, model.ScopeFull},
	} {
		for _, role := range tc.roles {
			if err := service.AssignRole(stores.Shared, tenant, tc.user, role); err != nil {
				t.Fatal(err)
			}
		}
		ops, err := service.EffectiveOperations(tenant, tc.user)
		if err != nil {
			t.Fatal(err)
		}
		if got := mustScope(t, ops, "product:read"); got != tc.want {
			t.Errorf("user %s: product:read = %v, want %v", tc.user, got, tc.want)
		}
	}
}

func TestRecordIDsUnionAcrossRoles(t *testing.T) {
	stores, tenant := setup(t)
	for role, ids := range map[string][]string{
		"r1": {"1", "2"},
		"r2": {"2", "3"},
	} {
		if err := service.CreateRole(tenant, role); err != nil {
			t.Fatal(err)
		}
		if err := service.SetRolePermission(stores.Shared, tenant, role, "product:read", model.ScopeRestricted); err != nil {
			t.Fatal(err)
		}
		if err := service.UpdateRoleRestrictedIDs(tenant, role, "product:read", ids, nil); err != nil {
			t.Fatal(err)
		}
		if err := service.AssignRole(stores.Shared, tenant, "pippo", role); err != nil {
			t.Fatal(err)
		}
	}

	ids, err := service.RecordIDs(tenant, "pippo", []string{"product:read", "invoice:read"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"product:read": {"1", "2", "3"}}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("RecordIDs = %v, want %v", ids, want)
	}
}

func TestRecordIDsUserOverrideReplacesRoleIDs(t *testing.T) {
	stores, tenant := setup(t)
	if err := service.CreateRole(tenant, "r1"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetRolePermission(stores.Shared, tenant, "r1", "product:read", model.ScopeRestricted); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateRoleRestrictedIDs(tenant, "r1", "product:read", []string{"1", "2"}, nil); err != nil {
		t.Fatal(err)
	}
	if err := service.AssignRole(stores.Shared, tenant, "pippo", "r1"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetUserPermission(stores.Shared, tenant, "pippo", "product:read", model.ScopeRestricted); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateUserRestrictedIDs(tenant, "pippo", "product:read", []string{"99"}, nil); err != nil {
		t.Fatal(err)
	}

	ids, err := service.RecordIDs(tenant, "pippo", []string{"product:read"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"product:read": {"99"}}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("RecordIDs = %v, want %v (user override, not role IDs)", ids, want)
	}
}

func TestRecordIDsOmitsNonRestricted(t *testing.T) {
	stores, tenant := setup(t)
	if err := service.SetUserPermission(stores.Shared, tenant, "pippo", "product:read", model.ScopeFull); err != nil {
		t.Fatal(err)
	}
	ids, err := service.RecordIDs(tenant, "pippo", []string{"product:read", "invoice:read"})
	if err != nil {
		t.Fatal(err)
	}
	if len(ids) != 0 {
		t.Errorf("FULL/unassigned operations must be omitted, got %v", ids)
	}
}

func TestRestrictedIDsAddRemove(t *testing.T) {
	stores, tenant := setup(t)
	if err := service.SetUserPermission(stores.Shared, tenant, "pippo", "product:read", model.ScopeRestricted); err != nil {
		t.Fatal(err)
	}
	if err := service.UpdateUserRestrictedIDs(tenant, "pippo", "product:read", []string{"1", "2", "3"}, nil); err != nil {
		t.Fatal(err)
	}
	// Adding an existing ID is idempotent; removal drops it.
	if err := service.UpdateUserRestrictedIDs(tenant, "pippo", "product:read", []string{"2"}, []string{"3"}); err != nil {
		t.Fatal(err)
	}
	ids, err := service.RecordIDs(tenant, "pippo", []string{"product:read"})
	if err != nil {
		t.Fatal(err)
	}
	want := map[string][]string{"product:read": {"1", "2"}}
	if !reflect.DeepEqual(ids, want) {
		t.Errorf("RecordIDs = %v, want %v", ids, want)
	}
}

func TestAuthorizeRequiresFull(t *testing.T) {
	stores, tenant := setup(t)
	if err := service.SetUserPermission(stores.Shared, tenant, "pippo", "product:read", model.ScopeRestricted); err != nil {
		t.Fatal(err)
	}
	ok, err := service.Authorize(tenant, "pippo", "product:read")
	if err != nil {
		t.Fatal(err)
	}
	if ok {
		t.Error("RESTRICTED must not satisfy Authorize")
	}
	if err := service.SetUserPermission(stores.Shared, tenant, "pippo", "product:read", model.ScopeFull); err != nil {
		t.Fatal(err)
	}
	ok, err = service.Authorize(tenant, "pippo", "product:read")
	if err != nil {
		t.Fatal(err)
	}
	if !ok {
		t.Error("FULL must satisfy Authorize")
	}
}

func TestTenantIsolation(t *testing.T) {
	stores, tenantA := setup(t)
	tenantB, _ := stores.Tenant("tenant-b")

	if err := service.CreateRole(tenantA, "manager"); err != nil {
		t.Fatal(err)
	}
	if err := service.SetRolePermission(stores.Shared, tenantA, "manager", "product:read", model.ScopeFull); err != nil {
		t.Fatal(err)
	}
	if err := service.AssignRole(stores.Shared, tenantA, "pippo", "manager"); err != nil {
		t.Fatal(err)
	}

	opsB, err := service.EffectiveOperations(tenantB, "pippo")
	if err != nil {
		t.Fatal(err)
	}
	if len(opsB) != 0 {
		t.Errorf("permissions must not leak across tenants, got %v", opsB)
	}
	if _, err := service.GetRole(tenantB, "manager"); err == nil {
		t.Error("role created in tenant-a must not exist in tenant-b")
	}
}
