# Multi-Tenancy

Sforza isolates tenants **physically**: each tenant has its own database,
not a `tenant_id` column.

## Shared database

Holds entities that are global by nature:

- **Resources** — all registered resources.
- **Operations** — all registered operations.
- **Users** — lazily provisioned principals, keyed by OIDC `sub`.

## Tenant databases

Each tenant database holds everything authorization-specific:

- Roles.
- User–role assignments.
- Role `(operation, scope)` permissions and their restricted IDs.
- User `(operation, scope)` overrides and their restricted IDs.

A role named `manager` in `tenant-a` and one in `tenant-b` are unrelated
objects; permissions granted in one tenant are invisible in every other.
This also applies to [meta permissions](meta-authorization.md): being an
administrator of `tenant-a` grants nothing in `tenant-b`.

## Selecting the tenant

Every API call must carry the tenant header:

```http
X-Tenant-ID: tenant-a
```

The set of valid tenants is exactly the set declared under
`storage.tenants` in the [configuration](configuration.md):

```yaml
storage:
  tenants:
    tenant-a:
      driver: postgres
      dsn: ${TENANT_A_DSN}
    tenant-b:
      driver: postgres
      dsn: ${TENANT_B_DSN}
```

A missing header is rejected with `400`; an undeclared tenant with `404`.
Adding a tenant is a configuration change followed by a restart — Sforza
migrates the new database automatically at startup.
