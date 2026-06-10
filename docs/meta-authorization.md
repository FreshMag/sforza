# Meta Authorization

Sforza authorizes access to its own administrative APIs using the same
SFBAC model — Sforza is a client of itself. The entities involved are
ordinary resources and operations, registered automatically at startup.

## Meta resources and operations

| Resource | Operation | Grants |
|---|---|---|
| `role` | `role:read` | Read roles. |
| `role` | `role:write` | Create / update / delete roles. |
| `role` | `role:assign` | Assign roles to users. |
| `operation` | `operation:read` | Read operations. |
| `operation` | `operation:write` | Create / update / delete operations. |
| `operation` | `operation:assign` | Assign `(operation, scope)` to roles or users. |
| `resource` | `resource:read` | Read resources. |
| `resource` | `resource:write` | Create / update / delete resources. |
| `user` | `user:read` | Read users and their effective permissions. |

The [API reference](api.md) lists the exact meta operation each endpoint
requires.

!!! warning "FULL scope is required"
    Administrative endpoints operate on whole collections, so a meta
    permission only takes effect at `FULL` scope. `RESTRICTED` and `EMPTY`
    grants on meta operations deny access.

## Tenant scoping

Meta permissions live in tenant databases like any other permission. A
user with `role:write` in `tenant-a` cannot touch roles in `tenant-b`
unless granted there too.

## Bootstrap administrator

On startup Sforza:

1. registers the meta resources and operations in the shared database;
2. creates the `authorization:admin` role in **every** tenant, containing
   every meta operation at `FULL` scope;
3. creates the user configured as `bootstrap.admin-sub` and assigns it the
   role in every tenant.

```yaml
bootstrap:
  admin-sub: admin-user-sub
```

The process is idempotent — restarting never duplicates anything, and the
admin role is re-asserted even if it was modified.

## Delegating administration

Because meta operations are ordinary operations, delegation is just
another grant. Give a team lead read-only visibility:

```bash
curl -X PUT -H 'X-Tenant-ID: tenant-a' ... \
     -d '{"scope":"FULL"}' \
     localhost:8080/api/v1/roles/team-lead/permissions/role:read
```

Or query which meta operations a user holds:

```bash
GET /api/v1/me/meta-operations
GET /api/v1/users/{sub}/meta-operations   # requires user:read
```
