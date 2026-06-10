# Sforza

Sforza is a centralized authorization service implementing **SFBAC**
(Scoped Functions-Based Access Control), a variation of RBAC where every
permission is an `(operation, scope)` pair. It is designed for microservice
ecosystems: multi-tenant, OIDC-authenticated (Keycloak-first), bootstrapped
from human-readable YAML files, and distributed as a Docker image.

## Concepts

| Concept | Meaning |
|---|---|
| **Resource** | A logical domain entity (`product`, `invoice`). Global, stored in the shared database. |
| **Operation** | An action on a resource, named `resource:action` (`product:read`). Global. |
| **Scope** | Visibility attached to an operation grant: `FULL` (all records), `EMPTY` (no records), `RESTRICTED` (an explicit set of record IDs). |
| **Role** | A named set of `(operation, scope)` pairs, per tenant. |
| **User override** | An `(operation, scope)` pair assigned directly to a user; it always beats role grants for that operation. |

Record IDs are stored and returned as strings, so integer IDs and UUIDs both
work (`[1, 2, 3]` in YAML becomes `["1", "2", "3"]`).

### Effective permission resolution

For each operation, in a given tenant:

1. If the user has a direct permission for the operation, that wins —
   including an `EMPTY` override silencing a `FULL` role grant.
2. Otherwise role grants are combined; the widest scope wins:
   `FULL > RESTRICTED > EMPTY`. When several roles grant `RESTRICTED`,
   their ID sets are **unioned**.
3. No assignment at all means **not allowed** (deny-by-default; the
   operation is simply absent from the effective set).

Restricted IDs follow the same precedence: a user-level `RESTRICTED`
permission uses only the user's ID set, never the roles' sets.

## Multi-tenancy

The shared database holds resources, operations and users (lazily
provisioned on first contact). Each tenant has its own database holding
roles, assignments, permissions and restricted IDs — tenant isolation is
physical, not a `WHERE` clause.

Every API call selects the active tenant with the `X-Tenant-ID` header.
Only tenants declared in the configuration are accepted.

## Authentication

With `auth.enabled: true` (the default) every request must carry:

- `Authorization: Bearer <JWT>` — validated against the configured OIDC
  issuer (discovery + JWKS, audience optionally enforced);
- `X-User-Sub` — must equal the JWT `sub` claim.

Users are lazily created in the shared database on first authenticated
request. For development and tests:

```yaml
auth:
  enabled: false
  default-sub: test-user
```

In this mode the `X-User-Sub` header (when present) selects the caller,
falling back to `default-sub`.

## Meta authorization — Sforza as a client of itself

Administrative APIs are authorized through the same SFBAC model, using meta
resources `role`, `operation`, `resource`, `user` and the operations
`role:read|write|assign`, `operation:read|write|assign`,
`resource:read|write`, `user:read`.

Admin endpoints require the corresponding meta operation with **FULL**
scope in the active tenant; `RESTRICTED` and `EMPTY` deny (admin APIs
operate on whole collections).

On startup Sforza creates the meta model, the `authorization:admin` role in
every tenant (every meta operation at `FULL`), and assigns it to the
configured `bootstrap.admin-sub`.

## API

All endpoints are under `/api/v1`, require authentication and the
`X-Tenant-ID` header. Errors are returned as `{"error": "..."}`.

### Self-service permission queries

| Method & path | Description |
|---|---|
| `GET /me/operations` | Effective `[{operation, scope}]` for the caller (never includes restricted IDs). |
| `GET /me/record-ids?operations=a,b` | Accessible record IDs per requested operation; only operations whose effective scope is `RESTRICTED` appear. |
| `GET /me/meta-operations` | Effective meta operations for the caller. |

### Administration

| Method & path | Required meta op | Description |
|---|---|---|
| `GET /resources` | `resource:read` | List resources. |
| `POST /resources` | `resource:write` | Create resource `{"name": "product"}`. |
| `DELETE /resources/{name}` | `resource:write` | Delete resource and its operations. |
| `GET /operations` | `operation:read` | List operations. |
| `POST /operations` | `operation:write` | Create operation `{"name": "product:read"}` (parent resource auto-created). |
| `DELETE /operations/{name}` | `operation:write` | Delete operation. |
| `GET /users` | `user:read` | List provisioned users. |
| `GET /users/{sub}/operations` | `user:read` | Effective operations of a user. |
| `GET /users/{sub}/meta-operations` | `user:read` | Effective meta operations of a user. |
| `GET /users/{sub}/roles` | `role:read` | Roles assigned to a user. |
| `PUT /users/{sub}/permissions/{op}` | `operation:assign` | Set user override `{"scope": "FULL"}`. |
| `DELETE /users/{sub}/permissions/{op}` | `operation:assign` | Remove user override (and its IDs). |
| `POST /users/{sub}/permissions/{op}/ids` | `operation:assign` | `{"add": ["1"], "remove": ["2"]}`. |
| `GET /roles` | `role:read` | List roles in the tenant. |
| `GET /roles/{name}` | `role:read` | Role with its permissions. |
| `POST /roles` | `role:write` | Create role `{"name": "manager"}`. |
| `PUT /roles/{name}` | `role:write` | Rename role `{"name": "new-name"}`. |
| `DELETE /roles/{name}` | `role:write` | Delete role, its permissions, IDs and assignments. |
| `POST /roles/{name}/assignments/{sub}` | `role:assign` | Assign role to user. |
| `DELETE /roles/{name}/assignments/{sub}` | `role:assign` | Unassign role. |
| `PUT /roles/{name}/permissions/{op}` | `operation:assign` | Set role permission `{"scope": "RESTRICTED"}`. |
| `DELETE /roles/{name}/permissions/{op}` | `operation:assign` | Remove role permission (and its IDs). |
| `POST /roles/{name}/permissions/{op}/ids` | `operation:assign` | `{"add": [...], "remove": [...]}`. |
| `POST /admin/sync` | all write/assign meta ops | Re-read and apply the bootstrap YAML files at runtime. |

`GET /healthz` is unauthenticated.

Example:

```bash
curl -H "Authorization: Bearer $TOKEN" \
     -H "X-User-Sub: john" \
     -H "X-Tenant-ID: tenant-a" \
     http://localhost:8080/api/v1/me/operations
# [{"operation":"invoice:read","scope":"FULL"},
#  {"operation":"product:write","scope":"RESTRICTED"}]
```

## Configuration

Loaded from `-config <path>` or `$SFORZA_CONFIG` (default `sforza.yaml`).
`${VAR}` references are expanded from the environment, which is how Docker
deployments inject DSNs and secrets. See
[configs/sforza.example.yaml](configs/sforza.example.yaml) and
[configs/sforza.dev.yaml](configs/sforza.dev.yaml).

```yaml
server:
  address: ":8080"
auth:
  enabled: true
  issuer: https://keycloak.example.com/realms/myrealm
  audience: sforza        # optional; empty skips the audience check
bootstrap:
  admin-sub: admin-user-sub
  files:
    - /etc/sforza/bootstrap/*.yaml
storage:
  shared:
    driver: postgres      # or sqlite
    dsn: ${SHARED_DSN}
  tenants:                # the full set of accepted X-Tenant-ID values
    tenant-a:
      driver: postgres
      dsn: ${TENANT_A_DSN}
```

## Bootstrap YAML

Each microservice contributes a file describing what it needs (see
[bootstrap/example.yaml](bootstrap/example.yaml)):

```yaml
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
          ids: [10, 15, 42]
    users:
      john:
        roles: [manager]
        permissions:
          invoice:approve: FULL
```

Synchronization runs at startup and on `POST /api/v1/admin/sync`. It is
**additive and idempotent**: declared entities are created or updated to
the declared state (scopes are upserted, restricted IDs are ensured
present); anything not mentioned is left untouched, since other services
may own it. Operations referenced by permissions are registered
automatically, so each file stays self-contained.

## Running

```bash
# Development (SQLite, auth disabled)
mkdir -p data && go run ./cmd/sforza -config configs/sforza.dev.yaml

# Full stack (Postgres + Sforza)
docker compose up --build
```

Images are published to GHCR by CI on every push to the default branch and
on `v*` tags: `ghcr.io/<owner>/sforza`.

## Development

```bash
go build ./...
go vet ./...
go test -race -cover ./...
```

The test suite covers permission resolution (overrides, multi-role merge,
ID unions), deny-by-default, tenant isolation, meta authorization, the full
admin API surface, OIDC token validation against a fake provider (expired,
forged, wrong-audience and mismatched-sub tokens), bootstrap idempotency
and configuration validation. Tests run against SQLite and need no external
services; the storage layer is identical for Postgres.

### Layout

```
cmd/sforza          entrypoint
internal/config     YAML + env configuration
internal/model      domain types (scopes, meta model)
internal/store      GORM models, shared + per-tenant databases
internal/service    permission resolution, administration, bootstrap sync
internal/auth       OIDC and development authenticators
internal/api        HTTP routing, middleware, handlers
bootstrap/          example microservice bootstrap file
configs/            example service configurations
deploy/             docker-compose support files
```
