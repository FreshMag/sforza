# Bootstrap YAML

In a microservice ecosystem each service knows which resources, operations
and default roles it needs. Sforza lets every service contribute a
human-readable YAML file; the files are applied at startup and can be
re-applied at runtime via `POST /api/v1/admin/sync`.

## File format

```yaml
# shop-service.yaml
resources:
  - product
  - invoice

operations:
  - product:read
  - product:write
  - invoice:read
  - invoice:approve

tenants:
  tenant-a:
    roles:
      manager:
        product:read: FULL
        invoice:read: FULL
        product:write:
          scope: RESTRICTED
          ids: [10, 15, 42]
      viewer:
        product:read: FULL
        invoice:read: EMPTY
    users:
      john:
        roles:
          - manager
        permissions:
          invoice:approve: FULL
```

Permissions accept two forms:

```yaml
product:read: FULL          # bare scope
product:write:              # scope with restricted IDs
  scope: RESTRICTED
  ids: [10, 15, 42]
```

## Semantics

Synchronization is **additive and idempotent**:

- Resources and operations are created when missing; operation names must
  have the `resource:action` form and the parent resource is derived from
  the prefix.
- Operations referenced by role or user permissions are registered
  automatically, so a file is self-contained even without an
  `operations:` list.
- Role permission scopes are **upserted** — re-declaring a permission with
  a different scope updates it.
- Restricted IDs are ensured present; IDs added through the API but not
  declared in YAML are left alone.
- Nothing is ever deleted: entities not mentioned in any file are
  untouched, since other services may own them. Removal is an explicit
  [API](api.md) action.
- Every tenant referenced by a file must be declared in the service
  configuration, otherwise synchronization fails.

Running the same files any number of times produces the same state.

## Runtime refresh

```bash
curl -X POST -H 'X-Tenant-ID: tenant-a' \
     -H "Authorization: Bearer $TOKEN" -H "X-User-Sub: $ADMIN" \
     localhost:8080/api/v1/admin/sync
# {"synced-files": 3}
```

The endpoint re-reads every glob in `bootstrap.files` and applies the
result. It requires `resource:write`, `operation:write`, `role:write`,
`role:assign` and `operation:assign` — everything synchronization can do —
in the tenant named by the request.
