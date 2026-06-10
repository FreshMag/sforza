// Package config loads and validates the Sforza service configuration.
package config

import (
	"fmt"
	"os"
	"strings"

	"gopkg.in/yaml.v3"
)

// Config is the root service configuration.
type Config struct {
	Server    Server    `yaml:"server"`
	Auth      Auth      `yaml:"auth"`
	Bootstrap Bootstrap `yaml:"bootstrap"`
	Storage   Storage   `yaml:"storage"`
}

// Server holds HTTP server settings.
type Server struct {
	Address string `yaml:"address"`
}

// Auth holds OIDC authentication settings. Enabled defaults to true so that
// security is opt-out, never accidentally off.
type Auth struct {
	Enabled    *bool  `yaml:"enabled"`
	Issuer     string `yaml:"issuer"`
	Audience   string `yaml:"audience"`
	DefaultSub string `yaml:"default-sub"`
}

// IsEnabled reports whether authentication is enabled (default true).
func (a Auth) IsEnabled() bool {
	return a.Enabled == nil || *a.Enabled
}

// Bootstrap holds startup synchronization settings.
type Bootstrap struct {
	AdminSub string   `yaml:"admin-sub"`
	Files    []string `yaml:"files"`
}

// DB is a single database connection definition.
type DB struct {
	Driver string `yaml:"driver"`
	DSN    string `yaml:"dsn"`
}

// Storage declares the shared database and one database per tenant. The keys
// of Tenants are the full set of tenant IDs the service accepts.
type Storage struct {
	Shared  DB            `yaml:"shared"`
	Tenants map[string]DB `yaml:"tenants"`
}

// Load reads, env-expands, parses and validates the configuration file at
// path. Environment variables are expanded with ${VAR} / $VAR syntax, which
// is how Docker deployments inject secrets.
func Load(path string) (*Config, error) {
	raw, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}
	return Parse(os.ExpandEnv(string(raw)))
}

// Parse parses and validates a YAML configuration document.
func Parse(doc string) (*Config, error) {
	cfg := &Config{}
	if err := yaml.Unmarshal([]byte(doc), cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	cfg.applyDefaults()
	if err := cfg.validate(); err != nil {
		return nil, err
	}
	return cfg, nil
}

func (c *Config) applyDefaults() {
	if c.Server.Address == "" {
		c.Server.Address = ":8080"
	}
	if !c.Auth.IsEnabled() && c.Auth.DefaultSub == "" {
		c.Auth.DefaultSub = "test-user"
	}
}

// Drivers supported by the store layer.
var validDrivers = map[string]bool{
	"sqlite":   true,
	"postgres": true,
	"mysql":    true,
	"json":     true,
}

func (c *Config) validate() error {
	if c.Auth.IsEnabled() && c.Auth.Issuer == "" {
		return fmt.Errorf("auth.issuer is required when authentication is enabled")
	}
	if c.Storage.Shared.Driver == "" || c.Storage.Shared.DSN == "" {
		return fmt.Errorf("storage.shared.driver and storage.shared.dsn are required")
	}
	if !validDrivers[strings.ToLower(c.Storage.Shared.Driver)] {
		return fmt.Errorf("storage.shared: unsupported driver %q (supported: sqlite, postgres, mysql, json)", c.Storage.Shared.Driver)
	}
	if len(c.Storage.Tenants) == 0 {
		return fmt.Errorf("at least one tenant must be declared under storage.tenants")
	}
	for id, db := range c.Storage.Tenants {
		if db.Driver == "" || db.DSN == "" {
			return fmt.Errorf("tenant %q: driver and dsn are required", id)
		}
		if !validDrivers[strings.ToLower(db.Driver)] {
			return fmt.Errorf("tenant %q: unsupported driver %q (supported: sqlite, postgres, mysql, json)", id, db.Driver)
		}
	}
	if c.Bootstrap.AdminSub == "" {
		return fmt.Errorf("bootstrap.admin-sub is required")
	}
	return nil
}
