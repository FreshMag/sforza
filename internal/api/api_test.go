package api_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/FreshMag/sforza/internal/api"
	"github.com/FreshMag/sforza/internal/auth"
	"github.com/FreshMag/sforza/internal/config"
	"github.com/FreshMag/sforza/internal/model"
	"github.com/FreshMag/sforza/internal/service"
	"github.com/FreshMag/sforza/internal/store"
	"github.com/FreshMag/sforza/internal/testutil"
)

type testEnv struct {
	t      *testing.T
	srv    *httptest.Server
	stores *store.Stores
	cfg    *config.Config
}

func newEnv(t *testing.T) *testEnv {
	return newEnvDriver(t, "sqlite")
}

func newEnvDriver(t *testing.T, driver string) *testEnv {
	t.Helper()
	stores := testutil.NewStoresDriver(t, driver)
	if err := service.BootstrapMeta(stores, testutil.AdminSub); err != nil {
		t.Fatal(err)
	}
	disabled := false
	cfg := &config.Config{
		Auth:      config.Auth{Enabled: &disabled, DefaultSub: "test-user"},
		Bootstrap: config.Bootstrap{AdminSub: testutil.AdminSub},
	}
	server := api.New(cfg, stores, auth.Static{DefaultSub: cfg.Auth.DefaultSub})
	srv := httptest.NewServer(server.Router())
	t.Cleanup(srv.Close)
	return &testEnv{t: t, srv: srv, stores: stores, cfg: cfg}
}

// do performs a request as the given subject inside the given tenant and
// decodes the JSON response into out (when out is non-nil).
func (e *testEnv) do(method, path, sub, tenant string, body, out any) int {
	e.t.Helper()
	var reader io.Reader
	if body != nil {
		raw, err := json.Marshal(body)
		if err != nil {
			e.t.Fatal(err)
		}
		reader = bytes.NewReader(raw)
	}
	req, err := http.NewRequest(method, e.srv.URL+path, reader)
	if err != nil {
		e.t.Fatal(err)
	}
	if sub != "" {
		req.Header.Set(auth.HeaderUserSub, sub)
	}
	if tenant != "" {
		req.Header.Set(api.HeaderTenantID, tenant)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		e.t.Fatal(err)
	}
	defer resp.Body.Close()
	raw, _ := io.ReadAll(resp.Body)
	if out != nil {
		if err := json.Unmarshal(raw, out); err != nil {
			e.t.Fatalf("decode %s %s response %q: %v", method, path, raw, err)
		}
	}
	return resp.StatusCode
}

func (e *testEnv) admin(method, path string, body, out any) int {
	return e.do(method, path, testutil.AdminSub, "tenant-a", body, out)
}

func (e *testEnv) check(status, want int, what string) {
	e.t.Helper()
	if status != want {
		e.t.Fatalf("%s: status = %d, want %d", what, status, want)
	}
}

func TestHealth(t *testing.T) {
	e := newEnv(t)
	resp, err := http.Get(e.srv.URL + "/healthz")
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("healthz = %d", resp.StatusCode)
	}
}

func TestTenantHeaderRequired(t *testing.T) {
	e := newEnv(t)
	e.check(e.do("GET", "/api/v1/me/operations", "pippo", "", nil, nil),
		http.StatusBadRequest, "missing tenant header")
	e.check(e.do("GET", "/api/v1/me/operations", "pippo", "ghost", nil, nil),
		http.StatusNotFound, "unknown tenant")
}

func TestUnauthenticatedRejected(t *testing.T) {
	stores := testutil.NewStores(t)
	disabled := false
	cfg := &config.Config{Auth: config.Auth{Enabled: &disabled}}
	// No default subject and no X-User-Sub header -> 401.
	srv := httptest.NewServer(api.New(cfg, stores, auth.Static{}).Router())
	defer srv.Close()
	req, _ := http.NewRequest("GET", srv.URL+"/api/v1/me/operations", nil)
	req.Header.Set(api.HeaderTenantID, "tenant-a")
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusUnauthorized {
		t.Fatalf("status = %d, want 401", resp.StatusCode)
	}
}

func TestAdminEndToEnd(t *testing.T) {
	// The full admin flow must behave identically on every embedded backend.
	for _, driver := range testutil.Drivers {
		t.Run(driver, func(t *testing.T) {
			testAdminEndToEnd(t, newEnvDriver(t, driver))
		})
	}
}

func testAdminEndToEnd(t *testing.T, e *testEnv) {

	// Register a resource and operations.
	e.check(e.admin("POST", "/api/v1/resources", map[string]string{"name": "product"}, nil),
		http.StatusCreated, "create resource")
	e.check(e.admin("POST", "/api/v1/resources", map[string]string{"name": "product"}, nil),
		http.StatusConflict, "duplicate resource")
	e.check(e.admin("POST", "/api/v1/operations", map[string]string{"name": "product:read"}, nil),
		http.StatusCreated, "create operation")
	e.check(e.admin("POST", "/api/v1/operations", map[string]string{"name": "badname"}, nil),
		http.StatusBadRequest, "operation without resource prefix")

	// Create a role and grant (operation, scope).
	e.check(e.admin("POST", "/api/v1/roles", map[string]string{"name": "manager"}, nil),
		http.StatusCreated, "create role")
	e.check(e.admin("PUT", "/api/v1/roles/manager/permissions/product:read",
		map[string]string{"scope": "RESTRICTED"}, nil),
		http.StatusNoContent, "set role permission")
	e.check(e.admin("PUT", "/api/v1/roles/manager/permissions/ghost:read",
		map[string]string{"scope": "FULL"}, nil),
		http.StatusNotFound, "permission on unregistered operation")
	e.check(e.admin("PUT", "/api/v1/roles/manager/permissions/product:read",
		map[string]string{"scope": "PARTIAL"}, nil),
		http.StatusBadRequest, "invalid scope")
	e.check(e.admin("POST", "/api/v1/roles/manager/permissions/product:read/ids",
		map[string][]string{"add": {"10", "15", "42"}}, nil),
		http.StatusNoContent, "add restricted ids")

	// Assign the role.
	e.check(e.admin("POST", "/api/v1/roles/manager/assignments/pippo", nil, nil),
		http.StatusNoContent, "assign role")

	// pippo sees the effective operation without IDs...
	var ops []model.OperationScope
	e.check(e.do("GET", "/api/v1/me/operations", "pippo", "tenant-a", nil, &ops),
		http.StatusOK, "me/operations")
	want := []model.OperationScope{{Operation: "product:read", Scope: model.ScopeRestricted}}
	if !reflect.DeepEqual(ops, want) {
		t.Fatalf("pippo ops = %v, want %v", ops, want)
	}

	// ...and fetches the IDs separately.
	var ids map[string][]string
	e.check(e.do("GET", "/api/v1/me/record-ids?operations=product:read", "pippo", "tenant-a", nil, &ids),
		http.StatusOK, "me/record-ids")
	if !reflect.DeepEqual(ids["product:read"], []string{"10", "15", "42"}) {
		t.Fatalf("record ids = %v", ids)
	}

	// User override beats the role.
	e.check(e.admin("PUT", "/api/v1/users/pippo/permissions/product:read",
		map[string]string{"scope": "FULL"}, nil),
		http.StatusNoContent, "user override")
	e.check(e.do("GET", "/api/v1/me/operations", "pippo", "tenant-a", nil, &ops),
		http.StatusOK, "me/operations after override")
	if ops[0].Scope != model.ScopeFull {
		t.Fatalf("override not applied: %v", ops)
	}

	// Admin views pippo through the user endpoints.
	e.check(e.admin("GET", "/api/v1/users/pippo/operations", nil, &ops),
		http.StatusOK, "users/{sub}/operations")
	if len(ops) != 1 || ops[0].Scope != model.ScopeFull {
		t.Fatalf("admin view of pippo = %v", ops)
	}
	var roles []string
	e.check(e.admin("GET", "/api/v1/users/pippo/roles", nil, &roles),
		http.StatusOK, "users/{sub}/roles")
	if !reflect.DeepEqual(roles, []string{"manager"}) {
		t.Fatalf("pippo roles = %v", roles)
	}

	// Remove override and unassign: deny-by-default returns.
	e.check(e.admin("DELETE", "/api/v1/users/pippo/permissions/product:read", nil, nil),
		http.StatusNoContent, "remove override")
	e.check(e.admin("DELETE", "/api/v1/roles/manager/assignments/pippo", nil, nil),
		http.StatusNoContent, "unassign role")
	e.check(e.do("GET", "/api/v1/me/operations", "pippo", "tenant-a", nil, &ops),
		http.StatusOK, "me/operations after cleanup")
	if len(ops) != 0 {
		t.Fatalf("pippo should have no operations, got %v", ops)
	}
}

func TestMetaAuthorization(t *testing.T) {
	e := newEnv(t)

	// A user with no meta permissions is denied on every admin endpoint.
	for _, probe := range []struct{ method, path string }{
		{"GET", "/api/v1/roles"},
		{"POST", "/api/v1/roles"},
		{"GET", "/api/v1/resources"},
		{"POST", "/api/v1/resources"},
		{"GET", "/api/v1/operations"},
		{"GET", "/api/v1/users"},
		{"POST", "/api/v1/admin/sync"},
	} {
		status := e.do(probe.method, probe.path, "intruder", "tenant-a", map[string]string{"name": "x"}, nil)
		if status != http.StatusForbidden {
			t.Errorf("%s %s as intruder: status = %d, want 403", probe.method, probe.path, status)
		}
	}

	// Meta permissions can be granted like any other: give pippo role:read.
	e.check(e.admin("POST", "/api/v1/roles", map[string]string{"name": "auditor"}, nil),
		http.StatusCreated, "create auditor")
	e.check(e.admin("PUT", "/api/v1/roles/auditor/permissions/role:read",
		map[string]string{"scope": "FULL"}, nil),
		http.StatusNoContent, "grant role:read")
	e.check(e.admin("POST", "/api/v1/roles/auditor/assignments/pippo", nil, nil),
		http.StatusNoContent, "assign auditor")

	e.check(e.do("GET", "/api/v1/roles", "pippo", "tenant-a", nil, nil),
		http.StatusOK, "pippo reads roles")
	e.check(e.do("POST", "/api/v1/roles", "pippo", "tenant-a", map[string]string{"name": "x"}, nil),
		http.StatusForbidden, "pippo cannot write roles")

	// RESTRICTED on a meta operation does not open admin APIs (FULL required).
	e.check(e.admin("PUT", "/api/v1/users/restricted-admin/permissions/role:read",
		map[string]string{"scope": "RESTRICTED"}, nil),
		http.StatusNoContent, "grant restricted role:read")
	e.check(e.do("GET", "/api/v1/roles", "restricted-admin", "tenant-a", nil, nil),
		http.StatusForbidden, "restricted meta scope denied")

	// pippo's meta-operations endpoint reflects the grant.
	var meta []model.OperationScope
	e.check(e.do("GET", "/api/v1/me/meta-operations", "pippo", "tenant-a", nil, &meta),
		http.StatusOK, "me/meta-operations")
	if len(meta) != 1 || meta[0].Operation != "role:read" {
		t.Fatalf("pippo meta ops = %v", meta)
	}
}

func TestMetaPermissionsAreTenantScoped(t *testing.T) {
	e := newEnv(t)
	// The bootstrap admin holds the admin role in both tenants.
	e.check(e.do("GET", "/api/v1/roles", testutil.AdminSub, "tenant-b", nil, nil),
		http.StatusOK, "admin in tenant-b")

	// A role granted only in tenant-a gives nothing in tenant-b.
	e.check(e.admin("POST", "/api/v1/roles", map[string]string{"name": "auditor"}, nil),
		http.StatusCreated, "create auditor in tenant-a")
	e.check(e.admin("PUT", "/api/v1/roles/auditor/permissions/role:read",
		map[string]string{"scope": "FULL"}, nil),
		http.StatusNoContent, "grant role:read")
	e.check(e.admin("POST", "/api/v1/roles/auditor/assignments/pippo", nil, nil),
		http.StatusNoContent, "assign auditor")
	e.check(e.do("GET", "/api/v1/roles", "pippo", "tenant-a", nil, nil),
		http.StatusOK, "pippo reads roles in tenant-a")
	e.check(e.do("GET", "/api/v1/roles", "pippo", "tenant-b", nil, nil),
		http.StatusForbidden, "pippo denied in tenant-b")
}

func TestTenantIsolationOverAPI(t *testing.T) {
	e := newEnv(t)
	e.check(e.admin("POST", "/api/v1/roles", map[string]string{"name": "only-in-a"}, nil),
		http.StatusCreated, "create role in tenant-a")

	var rolesB []map[string]any
	e.check(e.do("GET", "/api/v1/roles", testutil.AdminSub, "tenant-b", nil, &rolesB),
		http.StatusOK, "list roles in tenant-b")
	for _, r := range rolesB {
		if r["name"] == "only-in-a" {
			t.Fatal("role leaked into tenant-b")
		}
	}
}

func TestRoleLifecycle(t *testing.T) {
	e := newEnv(t)
	e.check(e.admin("POST", "/api/v1/operations", map[string]string{"name": "product:read"}, nil),
		http.StatusCreated, "create operation")
	e.check(e.admin("POST", "/api/v1/roles", map[string]string{"name": "temp"}, nil),
		http.StatusCreated, "create role")
	e.check(e.admin("PUT", "/api/v1/roles/temp/permissions/product:read",
		map[string]string{"scope": "FULL"}, nil),
		http.StatusNoContent, "grant permission")
	e.check(e.admin("POST", "/api/v1/roles/temp/assignments/pippo", nil, nil),
		http.StatusNoContent, "assign")

	// Rename and read back.
	e.check(e.admin("PUT", "/api/v1/roles/temp", map[string]string{"name": "renamed"}, nil),
		http.StatusOK, "rename role")
	var role struct {
		Name        string                 `json:"name"`
		Permissions []model.OperationScope `json:"permissions"`
	}
	e.check(e.admin("GET", "/api/v1/roles/renamed", nil, &role),
		http.StatusOK, "get renamed role")
	if len(role.Permissions) != 1 || role.Permissions[0].Operation != "product:read" {
		t.Fatalf("role detail = %+v", role)
	}

	// Deleting the role revokes pippo's access.
	e.check(e.admin("DELETE", "/api/v1/roles/renamed", nil, nil),
		http.StatusNoContent, "delete role")
	var ops []model.OperationScope
	e.check(e.do("GET", "/api/v1/me/operations", "pippo", "tenant-a", nil, &ops),
		http.StatusOK, "pippo ops after role deletion")
	if len(ops) != 0 {
		t.Fatalf("deleted role still grants %v", ops)
	}
	e.check(e.admin("GET", "/api/v1/roles/renamed", nil, nil),
		http.StatusNotFound, "deleted role gone")
}

func TestLazyProvisioning(t *testing.T) {
	e := newEnv(t)
	// Any authenticated call provisions the caller.
	e.check(e.do("GET", "/api/v1/me/operations", "fresh-user", "tenant-a", nil, nil),
		http.StatusOK, "fresh user request")
	var users []map[string]string
	e.check(e.admin("GET", "/api/v1/users", nil, &users), http.StatusOK, "list users")
	found := false
	for _, u := range users {
		if u["sub"] == "fresh-user" {
			found = true
		}
	}
	if !found {
		t.Fatalf("fresh-user not lazily provisioned: %v", users)
	}
}

func TestSyncEndpoint(t *testing.T) {
	e := newEnv(t)
	dir := t.TempDir()
	doc := `
operations:
  - product:read
tenants:
  tenant-a:
    roles:
      manager:
        product:read: FULL
    users:
      john:
        roles: [manager]
`
	if err := os.WriteFile(filepath.Join(dir, "svc.yaml"), []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	e.cfg.Bootstrap.Files = []string{filepath.Join(dir, "*.yaml")}

	e.check(e.do("POST", "/api/v1/admin/sync", "john", "tenant-a", nil, nil),
		http.StatusForbidden, "sync requires meta permissions")

	var result map[string]int
	e.check(e.admin("POST", "/api/v1/admin/sync", nil, &result), http.StatusOK, "sync as admin")
	if result["synced-files"] != 1 {
		t.Fatalf("synced-files = %d", result["synced-files"])
	}

	var ops []model.OperationScope
	e.check(e.do("GET", "/api/v1/me/operations", "john", "tenant-a", nil, &ops),
		http.StatusOK, "john ops after sync")
	if len(ops) != 1 || ops[0].Operation != "product:read" || ops[0].Scope != model.ScopeFull {
		t.Fatalf("john ops = %v", ops)
	}
}

func TestRecordIDsRequiresOperationsParam(t *testing.T) {
	e := newEnv(t)
	e.check(e.do("GET", "/api/v1/me/record-ids", "pippo", "tenant-a", nil, nil),
		http.StatusBadRequest, "record-ids without operations param")
}

func TestUserRestrictedIDsViaAPI(t *testing.T) {
	e := newEnv(t)
	e.check(e.admin("POST", "/api/v1/operations", map[string]string{"name": "invoice:read"}, nil),
		http.StatusCreated, "create operation")
	e.check(e.admin("PUT", "/api/v1/users/pippo/permissions/invoice:read",
		map[string]string{"scope": "RESTRICTED"}, nil),
		http.StatusNoContent, "set user permission")
	e.check(e.admin("POST", "/api/v1/users/pippo/permissions/invoice:read/ids",
		map[string][]string{"add": {"10", "20"}}, nil),
		http.StatusNoContent, "add ids")
	e.check(e.admin("POST", "/api/v1/users/pippo/permissions/invoice:read/ids",
		map[string][]string{"remove": {"10"}}, nil),
		http.StatusNoContent, "remove id")

	var ids map[string][]string
	e.check(e.do("GET", "/api/v1/me/record-ids?operations=invoice:read", "pippo", "tenant-a", nil, &ids),
		http.StatusOK, "record-ids")
	if !reflect.DeepEqual(ids["invoice:read"], []string{"20"}) {
		t.Fatalf("ids = %v, want [20]", ids)
	}

	// IDs on an operation the user has no permission for -> 404.
	status := e.admin("POST", fmt.Sprintf("/api/v1/users/%s/permissions/%s/ids", "pippo", "ghost:read"),
		map[string][]string{"add": {"1"}}, nil)
	e.check(status, http.StatusNotFound, "ids without permission")
}
