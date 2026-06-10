package service_test

import (
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/FreshMag/sforza/internal/model"
	"github.com/FreshMag/sforza/internal/service"
	"github.com/FreshMag/sforza/internal/testutil"
)

const bootstrapDoc = `
resources:
  - product
operations:
  - product:read
  - product:write
tenants:
  tenant-a:
    roles:
      manager:
        product:read: FULL
        product:write:
          scope: RESTRICTED
          ids: [10, 15]
    users:
      john:
        roles: [manager]
        permissions:
          invoice:approve: FULL
  tenant-b:
    roles:
      viewer:
        product:read: FULL
`

func loadFiles(t *testing.T, doc string) []service.BootstrapFile {
	t.Helper()
	path := filepath.Join(t.TempDir(), "bootstrap.yaml")
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	files, err := service.LoadBootstrapFiles([]string{path})
	if err != nil {
		t.Fatal(err)
	}
	return files
}

func TestSyncCreatesEverything(t *testing.T) {
	stores := testutil.NewStores(t)
	if err := service.Sync(stores, loadFiles(t, bootstrapDoc)); err != nil {
		t.Fatalf("Sync: %v", err)
	}

	resources, err := service.ListResources(stores.Shared)
	if err != nil {
		t.Fatal(err)
	}
	names := map[string]bool{}
	for _, r := range resources {
		names[r.Name] = true
	}
	// "invoice" comes from the referenced invoice:approve permission.
	for _, want := range []string{"product", "invoice"} {
		if !names[want] {
			t.Errorf("resource %q not created (have %v)", want, names)
		}
	}

	operations, err := service.ListOperations(stores.Shared)
	if err != nil {
		t.Fatal(err)
	}
	opNames := map[string]bool{}
	for _, o := range operations {
		opNames[o.Name] = true
	}
	for _, want := range []string{"product:read", "product:write", "invoice:approve"} {
		if !opNames[want] {
			t.Errorf("operation %q not created", want)
		}
	}

	tenantA, _ := stores.Tenant("tenant-a")
	ops, err := service.EffectiveOperations(tenantA, "john")
	if err != nil {
		t.Fatal(err)
	}
	want := []model.OperationScope{
		{Operation: "invoice:approve", Scope: model.ScopeFull},
		{Operation: "product:read", Scope: model.ScopeFull},
		{Operation: "product:write", Scope: model.ScopeRestricted},
	}
	if !reflect.DeepEqual(ops, want) {
		t.Errorf("john effective ops = %v, want %v", ops, want)
	}

	ids, err := service.RecordIDs(tenantA, "john", []string{"product:write"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids["product:write"], []string{"10", "15"}) {
		t.Errorf("restricted ids = %v, want [10 15]", ids["product:write"])
	}

	// tenant-b got only its own role; john has nothing there.
	tenantB, _ := stores.Tenant("tenant-b")
	opsB, err := service.EffectiveOperations(tenantB, "john")
	if err != nil {
		t.Fatal(err)
	}
	if len(opsB) != 0 {
		t.Errorf("tenant-b must not inherit tenant-a config, got %v", opsB)
	}
	if _, err := service.GetRole(tenantB, "viewer"); err != nil {
		t.Errorf("viewer role missing in tenant-b: %v", err)
	}
}

func TestSyncIsIdempotent(t *testing.T) {
	stores := testutil.NewStores(t)
	files := loadFiles(t, bootstrapDoc)
	for i := 0; i < 3; i++ {
		if err := service.Sync(stores, files); err != nil {
			t.Fatalf("Sync #%d: %v", i+1, err)
		}
	}

	operations, err := service.ListOperations(stores.Shared)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	for _, op := range operations {
		if op.Name == "product:read" {
			count++
		}
	}
	if count != 1 {
		t.Errorf("operation duplicated: count = %d", count)
	}
	tenantA, _ := stores.Tenant("tenant-a")
	ids, err := service.RecordIDs(tenantA, "john", []string{"product:write"})
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(ids["product:write"], []string{"10", "15"}) {
		t.Errorf("restricted ids duplicated or lost: %v", ids["product:write"])
	}
}

func TestSyncUpdatesScope(t *testing.T) {
	stores := testutil.NewStores(t)
	if err := service.Sync(stores, loadFiles(t, bootstrapDoc)); err != nil {
		t.Fatal(err)
	}
	updated := loadFiles(t, `
tenants:
  tenant-a:
    roles:
      manager:
        product:read: EMPTY
`)
	if err := service.Sync(stores, updated); err != nil {
		t.Fatal(err)
	}
	tenantA, _ := stores.Tenant("tenant-a")
	ops, err := service.EffectiveOperations(tenantA, "john")
	if err != nil {
		t.Fatal(err)
	}
	for _, os := range ops {
		if os.Operation == "product:read" && os.Scope != model.ScopeEmpty {
			t.Errorf("product:read = %v, want EMPTY after re-sync", os.Scope)
		}
	}
}

func TestSyncRejectsUnknownTenant(t *testing.T) {
	stores := testutil.NewStores(t)
	files := loadFiles(t, `
tenants:
  ghost-tenant:
    roles:
      r: {product:read: FULL}
`)
	if err := service.Sync(stores, files); err == nil {
		t.Fatal("Sync must fail for a tenant that is not configured")
	}
}

func TestSyncRejectsInvalidScope(t *testing.T) {
	stores := testutil.NewStores(t)
	files := loadFiles(t, `
tenants:
  tenant-a:
    roles:
      r: {product:read: SOMETIMES}
`)
	if err := service.Sync(stores, files); err == nil {
		t.Fatal("Sync must reject invalid scopes")
	}
}

func TestBootstrapMeta(t *testing.T) {
	stores := testutil.NewStores(t)
	for i := 0; i < 2; i++ { // idempotent
		if err := service.BootstrapMeta(stores, testutil.AdminSub); err != nil {
			t.Fatalf("BootstrapMeta #%d: %v", i+1, err)
		}
	}

	for _, tenantID := range testutil.Tenants {
		tenant, _ := stores.Tenant(tenantID)
		ops, err := service.MetaOperations(tenant, testutil.AdminSub)
		if err != nil {
			t.Fatal(err)
		}
		if len(ops) != len(model.MetaOperations) {
			t.Fatalf("tenant %s: admin has %d meta ops, want %d (%v)",
				tenantID, len(ops), len(model.MetaOperations), ops)
		}
		for _, os := range ops {
			if os.Scope != model.ScopeFull {
				t.Errorf("tenant %s: %s = %v, want FULL", tenantID, os.Operation, os.Scope)
			}
		}
		if _, err := service.GetRole(tenant, model.AdminRole); err != nil {
			t.Errorf("tenant %s: admin role missing: %v", tenantID, err)
		}
	}
}
