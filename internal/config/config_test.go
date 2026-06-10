package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

const validDoc = `
auth:
  enabled: false
bootstrap:
  admin-sub: admin
storage:
  shared:
    driver: sqlite
    dsn: shared.db
  tenants:
    tenant-a:
      driver: sqlite
      dsn: a.db
`

func TestParseValid(t *testing.T) {
	cfg, err := Parse(validDoc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if cfg.Server.Address != ":8080" {
		t.Errorf("default address = %q, want :8080", cfg.Server.Address)
	}
	if cfg.Auth.IsEnabled() {
		t.Error("auth should be disabled")
	}
	if cfg.Auth.DefaultSub != "test-user" {
		t.Errorf("default sub = %q, want test-user", cfg.Auth.DefaultSub)
	}
	if cfg.Bootstrap.AdminSub != "admin" {
		t.Errorf("admin sub = %q", cfg.Bootstrap.AdminSub)
	}
}

func TestAuthEnabledByDefault(t *testing.T) {
	doc := strings.Replace(validDoc, "auth:\n  enabled: false\n", "", 1)
	if _, err := Parse(doc); err == nil || !strings.Contains(err.Error(), "auth.issuer") {
		t.Fatalf("want issuer-required error, got %v", err)
	}
	doc = "auth:\n  issuer: https://idp.example.com\n" + doc
	cfg, err := Parse(doc)
	if err != nil {
		t.Fatalf("Parse: %v", err)
	}
	if !cfg.Auth.IsEnabled() {
		t.Error("auth must default to enabled")
	}
}

func TestValidationErrors(t *testing.T) {
	cases := []struct {
		name, doc, wantErr string
	}{
		{"missing shared", `
auth: {enabled: false}
bootstrap: {admin-sub: a}
storage:
  tenants:
    t: {driver: sqlite, dsn: x.db}
`, "storage.shared"},
		{"no tenants", `
auth: {enabled: false}
bootstrap: {admin-sub: a}
storage:
  shared: {driver: sqlite, dsn: x.db}
`, "tenant"},
		{"missing admin", `
auth: {enabled: false}
storage:
  shared: {driver: sqlite, dsn: x.db}
  tenants:
    t: {driver: sqlite, dsn: x.db}
`, "admin-sub"},
		{"tenant without dsn", `
auth: {enabled: false}
bootstrap: {admin-sub: a}
storage:
  shared: {driver: sqlite, dsn: x.db}
  tenants:
    t: {driver: sqlite}
`, `tenant "t"`},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := Parse(tc.doc)
			if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
				t.Fatalf("want error containing %q, got %v", tc.wantErr, err)
			}
		})
	}
}

func TestLoadExpandsEnv(t *testing.T) {
	t.Setenv("TEST_ADMIN_SUB", "env-admin")
	path := filepath.Join(t.TempDir(), "cfg.yaml")
	doc := strings.Replace(validDoc, "admin-sub: admin", "admin-sub: ${TEST_ADMIN_SUB}", 1)
	if err := os.WriteFile(path, []byte(doc), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg, err := Load(path)
	if err != nil {
		t.Fatalf("Load: %v", err)
	}
	if cfg.Bootstrap.AdminSub != "env-admin" {
		t.Errorf("admin sub = %q, want env-admin", cfg.Bootstrap.AdminSub)
	}
}
