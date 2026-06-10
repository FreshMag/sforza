package store_test

import (
	"errors"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/FreshMag/sforza/internal/config"
	"github.com/FreshMag/sforza/internal/model"
	"github.com/FreshMag/sforza/internal/store"
	"github.com/FreshMag/sforza/internal/testutil"
)

// TestDriverParity runs the same scenario against every embedded backend so
// the SQLite/GORM and JSON implementations cannot drift apart.
func TestDriverParity(t *testing.T) {
	for _, driver := range testutil.Drivers {
		t.Run(driver, func(t *testing.T) {
			stores := testutil.NewStoresDriver(t, driver)
			shared := stores.Shared
			tenant, _ := stores.Tenant("tenant-a")

			// Shared store: resources and operations.
			if err := shared.CreateResource("product"); err != nil {
				t.Fatal(err)
			}
			if err := shared.CreateResource("product"); !errors.Is(err, store.ErrConflict) {
				t.Fatalf("duplicate resource: err = %v, want ErrConflict", err)
			}
			if err := shared.CreateOperation("product:read", "product"); err != nil {
				t.Fatal(err)
			}
			// CreateOperation auto-creates a missing parent resource.
			if err := shared.CreateOperation("invoice:read", "invoice"); err != nil {
				t.Fatal(err)
			}
			resources, err := shared.ListResources()
			if err != nil {
				t.Fatal(err)
			}
			if want := []store.Resource{{Name: "invoice"}, {Name: "product"}}; !reflect.DeepEqual(resources, want) {
				t.Fatalf("resources = %v, want %v", resources, want)
			}
			if ok, _ := shared.OperationExists("product:read"); !ok {
				t.Fatal("product:read must exist")
			}
			if ok, _ := shared.OperationExists("ghost:read"); ok {
				t.Fatal("ghost:read must not exist")
			}

			// Users.
			if err := shared.EnsureUser("pippo"); err != nil {
				t.Fatal(err)
			}
			if err := shared.EnsureUser("pippo"); err != nil { // idempotent
				t.Fatal(err)
			}
			users, err := shared.ListUsers()
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(users, []store.User{{Sub: "pippo"}}) {
				t.Fatalf("users = %v", users)
			}

			// Roles.
			if err := tenant.CreateRole("manager"); err != nil {
				t.Fatal(err)
			}
			if err := tenant.CreateRole("manager"); !errors.Is(err, store.ErrConflict) {
				t.Fatalf("duplicate role: err = %v, want ErrConflict", err)
			}
			if err := tenant.CreateRole("viewer"); err != nil {
				t.Fatal(err)
			}

			// Permissions with upsert.
			if err := tenant.SetRolePermission("manager", "product:read", model.ScopeEmpty); err != nil {
				t.Fatal(err)
			}
			if err := tenant.SetRolePermission("manager", "product:read", model.ScopeRestricted); err != nil {
				t.Fatal(err)
			}
			if err := tenant.SetRolePermission("ghost", "product:read", model.ScopeFull); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("permission on missing role: err = %v, want ErrNotFound", err)
			}
			perms, err := tenant.RolePermissions("manager")
			if err != nil {
				t.Fatal(err)
			}
			want := []model.OperationScope{{Operation: "product:read", Scope: model.ScopeRestricted}}
			if !reflect.DeepEqual(perms, want) {
				t.Fatalf("permissions = %v, want %v (upsert)", perms, want)
			}

			// Restricted IDs: add is idempotent, union is deduplicated.
			if err := tenant.SetRolePermission("viewer", "product:read", model.ScopeRestricted); err != nil {
				t.Fatal(err)
			}
			if err := tenant.AddRoleRestrictedIDs("manager", "product:read", []string{"1", "2", "2"}); err != nil {
				t.Fatal(err)
			}
			if err := tenant.AddRoleRestrictedIDs("viewer", "product:read", []string{"2", "3"}); err != nil {
				t.Fatal(err)
			}
			ids, err := tenant.RoleRestrictedIDs([]string{"manager", "viewer"}, "product:read")
			if err != nil {
				t.Fatal(err)
			}
			if len(ids) != 3 {
				t.Fatalf("union ids = %v, want 3 distinct", ids)
			}
			if err := tenant.RemoveRoleRestrictedIDs("viewer", "product:read", []string{"3"}); err != nil {
				t.Fatal(err)
			}
			ids, _ = tenant.RoleRestrictedIDs([]string{"viewer"}, "product:read")
			if !reflect.DeepEqual(ids, []string{"2"}) {
				t.Fatalf("viewer ids after removal = %v, want [2]", ids)
			}

			// Assignments and grants.
			if err := tenant.AssignRole("pippo", "manager"); err != nil {
				t.Fatal(err)
			}
			if err := tenant.AssignRole("pippo", "manager"); err != nil { // idempotent
				t.Fatal(err)
			}
			if err := tenant.AssignRole("pippo", "ghost"); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("assign missing role: err = %v, want ErrNotFound", err)
			}
			roles, err := tenant.UserRoles("pippo")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(roles, []string{"manager"}) {
				t.Fatalf("roles = %v", roles)
			}
			grants, err := tenant.RoleGrants(roles)
			if err != nil {
				t.Fatal(err)
			}
			wantGrant := []store.RoleGrant{{Role: "manager", Operation: "product:read", Scope: model.ScopeRestricted}}
			if !reflect.DeepEqual(grants, wantGrant) {
				t.Fatalf("grants = %v, want %v", grants, wantGrant)
			}

			// User permissions and IDs.
			if err := tenant.SetUserPermission("pippo", "invoice:read", model.ScopeRestricted); err != nil {
				t.Fatal(err)
			}
			if err := tenant.AddUserRestrictedIDs("pippo", "invoice:read", []string{"9"}); err != nil {
				t.Fatal(err)
			}
			uids, err := tenant.UserRestrictedIDs("pippo", "invoice:read")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(uids, []string{"9"}) {
				t.Fatalf("user ids = %v", uids)
			}
			if err := tenant.RemoveUserPermission("pippo", "invoice:read"); err != nil {
				t.Fatal(err)
			}
			if err := tenant.RemoveUserPermission("pippo", "invoice:read"); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("remove missing user permission: err = %v, want ErrNotFound", err)
			}
			uids, _ = tenant.UserRestrictedIDs("pippo", "invoice:read")
			if len(uids) != 0 {
				t.Fatalf("user ids must cascade with the permission, got %v", uids)
			}

			// Rename keeps permissions and assignments.
			if err := tenant.RenameRole("manager", "boss"); err != nil {
				t.Fatal(err)
			}
			if err := tenant.RenameRole("viewer", "boss"); !errors.Is(err, store.ErrConflict) {
				t.Fatalf("rename onto existing role: err = %v, want ErrConflict", err)
			}
			roles, _ = tenant.UserRoles("pippo")
			if !reflect.DeepEqual(roles, []string{"boss"}) {
				t.Fatalf("roles after rename = %v, want [boss]", roles)
			}
			perms, _ = tenant.RolePermissions("boss")
			if len(perms) != 1 {
				t.Fatalf("permissions lost on rename: %v", perms)
			}

			// Delete cascades permissions, IDs and assignments.
			if err := tenant.DeleteRole("boss"); err != nil {
				t.Fatal(err)
			}
			roles, _ = tenant.UserRoles("pippo")
			if len(roles) != 0 {
				t.Fatalf("assignments must cascade on delete, got %v", roles)
			}
			if _, err := tenant.RolePermissions("boss"); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("deleted role: err = %v, want ErrNotFound", err)
			}

			// Tenant isolation.
			tenantB, _ := stores.Tenant("tenant-b")
			if ok, _ := tenantB.RoleExists("viewer"); ok {
				t.Fatal("role leaked into tenant-b")
			}

			// Deleting a resource removes its operations.
			if err := shared.DeleteResource("product"); err != nil {
				t.Fatal(err)
			}
			if ok, _ := shared.OperationExists("product:read"); ok {
				t.Fatal("operations must cascade with their resource")
			}
			if err := shared.DeleteResource("product"); !errors.Is(err, store.ErrNotFound) {
				t.Fatalf("delete missing resource: err = %v, want ErrNotFound", err)
			}
		})
	}
}

// TestJSONStorePersistence verifies that the JSON store survives a reopen
// from the same files.
func TestJSONStorePersistence(t *testing.T) {
	dir := t.TempDir()
	cfg := config.Storage{
		Shared: config.DB{Driver: "json", DSN: filepath.Join(dir, "shared.json")},
		Tenants: map[string]config.DB{
			"tenant-a": {Driver: "json", DSN: filepath.Join(dir, "tenant-a.json")},
		},
	}

	stores, err := store.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	tenant, _ := stores.Tenant("tenant-a")
	if err := stores.Shared.CreateOperation("product:read", "product"); err != nil {
		t.Fatal(err)
	}
	if err := tenant.CreateRole("manager"); err != nil {
		t.Fatal(err)
	}
	if err := tenant.SetRolePermission("manager", "product:read", model.ScopeRestricted); err != nil {
		t.Fatal(err)
	}
	if err := tenant.AddRoleRestrictedIDs("manager", "product:read", []string{"1", "2"}); err != nil {
		t.Fatal(err)
	}
	if err := tenant.AssignRole("john", "manager"); err != nil {
		t.Fatal(err)
	}

	// Reopen from the same files: everything must still be there.
	reopened, err := store.Open(cfg)
	if err != nil {
		t.Fatal(err)
	}
	if ok, _ := reopened.Shared.OperationExists("product:read"); !ok {
		t.Error("operation lost on reload")
	}
	tenant2, _ := reopened.Tenant("tenant-a")
	roles, err := tenant2.UserRoles("john")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(roles, []string{"manager"}) {
		t.Errorf("assignments lost on reload: %v", roles)
	}
	ids, err := tenant2.RoleRestrictedIDs([]string{"manager"}, "product:read")
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids, []string{"1", "2"}) {
		t.Errorf("restricted ids lost on reload: %v", ids)
	}
}
