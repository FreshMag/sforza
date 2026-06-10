# Configuration

Sforza reads a single YAML file, selected by (in order of precedence):

1. the `-config <path>` flag;
2. the `SFORZA_CONFIG` environment variable;
3. `./sforza.yaml`.

Any `${VAR}` (or `$VAR`) reference in the file is expanded from the
environment before parsing — the intended way to inject DSNs and secrets
in Docker deployments.

## Full reference

```yaml
server:
  address: ":8080"              # default :8080

auth:
  enabled: true                 # default true
  issuer: https://keycloak.example.com/realms/myrealm
  audience: sforza              # optional; empty skips the audience check
  default-sub: test-user        # used only when enabled: false

bootstrap:
  admin-sub: admin-user-sub     # required
  files:                        # glob patterns of bootstrap YAML files
    - /etc/sforza/bootstrap/*.yaml

storage:
  shared:                       # required
    driver: postgres            # postgres | mysql | sqlite | json
    dsn: ${SHARED_DSN}
  tenants:                      # at least one required; keys = tenant IDs
    tenant-a:
      driver: postgres
      dsn: ${TENANT_A_DSN}
    tenant-b:
      driver: sqlite
      dsn: data/tenant-b.db
```

## Fields

| Field | Required | Notes |
|---|---|---|
| `server.address` | no | Listen address, defaults to `:8080`. |
| `auth.enabled` | no | Defaults to `true` (security by default). |
| `auth.issuer` | when enabled | OIDC issuer URL; discovery must be reachable at startup. |
| `auth.audience` | no | Expected `aud` claim; empty disables the check. |
| `auth.default-sub` | no | Caller fallback in disabled mode; defaults to `test-user`. |
| `bootstrap.admin-sub` | yes | Subject of the bootstrap administrator. |
| `bootstrap.files` | no | Glob patterns applied at startup and on `POST /api/v1/admin/sync`. |
| `storage.shared` | yes | Shared database (resources, operations, users). |
| `storage.tenants` | yes | One entry per tenant; the keys are the accepted `X-Tenant-ID` values. |

## Drivers

| Driver | DSN example | Use |
|---|---|---|
| `postgres` | `host=db user=sforza password=... dbname=sforza_shared sslmode=disable` | Production. |
| `mysql` | `sforza:secret@tcp(db:3306)/sforza_shared?parseTime=true` | Production. |
| `sqlite` | `data/shared.db` | Development, tests, single-node setups. |
| `json` | `data/shared.json` | Development, tests, tiny single-node setups. |

Drivers can be mixed freely — e.g. a PostgreSQL shared store with one JSON
file per tenant.

For the SQL drivers, schema migration runs automatically and idempotently
for every database at startup, so adding a tenant is just a new
`storage.tenants` entry pointing at an empty database.

!!! note "The JSON store"
    The `json` driver keeps the whole dataset in memory and rewrites the
    backing file atomically on every change. The layout mirrors the
    bootstrap YAML (roles with permissions and restricted IDs, users with
    roles and overrides), so the file stays human-readable and diffable.
    It is not meant for multi-instance deployments or large datasets.
