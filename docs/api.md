# REST API

All endpoints live under `/api/v1`. Every request must be
[authenticated](authentication.md) and carry the `X-Tenant-ID` header
([multi-tenancy](multi-tenancy.md)). `GET /healthz` is the only
unauthenticated endpoint.

Errors are JSON: `{"error": "..."}` with conventional status codes —
`400` validation, `401` authentication, `403` missing meta permission,
`404` unknown entity/tenant, `409` duplicate.

Administrative endpoints require the listed
[meta operation](meta-authorization.md) at `FULL` scope in the active
tenant.

## Self-service queries

### `GET /me/operations`

Effective operations of the caller after combining role permissions and
user overrides. Restricted IDs are **not** included.

```json
[
  {"operation": "product:read", "scope": "FULL"},
  {"operation": "invoice:read", "scope": "RESTRICTED"}
]
```

### `GET /me/record-ids?operations=op1,op2`

Accessible record IDs for the requested operations. Only operations whose
effective scope is `RESTRICTED` appear in the response.

```json
{"product:read": ["1", "2", "3"], "invoice:read": ["10", "20"]}
```

### `GET /me/meta-operations`

Effective operations filtered to the meta authorization model.

## Resources

| Endpoint | Meta op | Body | Response |
|---|---|---|---|
| `GET /resources` | `resource:read` | — | `[{"name": "product"}]` |
| `POST /resources` | `resource:write` | `{"name": "product"}` | `201` |
| `DELETE /resources/{name}` | `resource:write` | — | `204` (operations of the resource are deleted too) |

## Operations

| Endpoint | Meta op | Body | Response |
|---|---|---|---|
| `GET /operations` | `operation:read` | — | `[{"name": "product:read", "resource": "product"}]` |
| `POST /operations` | `operation:write` | `{"name": "product:read"}` | `201` (parent resource auto-created) |
| `DELETE /operations/{name}` | `operation:write` | — | `204` |

## Users

| Endpoint | Meta op | Description |
|---|---|---|
| `GET /users` | `user:read` | All lazily provisioned users. |
| `GET /users/{sub}/operations` | `user:read` | Effective operations of a user. |
| `GET /users/{sub}/meta-operations` | `user:read` | Effective meta operations of a user. |
| `GET /users/{sub}/roles` | `role:read` | Role names assigned to a user. |

## Roles

| Endpoint | Meta op | Body |
|---|---|---|
| `GET /roles` | `role:read` | — |
| `GET /roles/{name}` | `role:read` | — (returns the role with its permissions) |
| `POST /roles` | `role:write` | `{"name": "manager"}` |
| `PUT /roles/{name}` | `role:write` | `{"name": "new-name"}` (rename) |
| `DELETE /roles/{name}` | `role:write` | — (cascades permissions, IDs, assignments) |
| `POST /roles/{name}/assignments/{sub}` | `role:assign` | — (idempotent; provisions the user) |
| `DELETE /roles/{name}/assignments/{sub}` | `role:assign` | — |

## Permission assignments

Operations must already be registered; scopes are upserted.

| Endpoint | Meta op | Body |
|---|---|---|
| `PUT /roles/{name}/permissions/{op}` | `operation:assign` | `{"scope": "FULL"}` |
| `DELETE /roles/{name}/permissions/{op}` | `operation:assign` | — (drops the permission's restricted IDs too) |
| `PUT /users/{sub}/permissions/{op}` | `operation:assign` | `{"scope": "RESTRICTED"}` |
| `DELETE /users/{sub}/permissions/{op}` | `operation:assign` | — |

Operation names contain `:` and are used directly in paths:
`PUT /api/v1/roles/manager/permissions/product:read`.

## Restricted IDs

| Endpoint | Meta op | Body |
|---|---|---|
| `POST /roles/{name}/permissions/{op}/ids` | `operation:assign` | `{"add": ["10"], "remove": ["15"]}` |
| `POST /users/{sub}/permissions/{op}/ids` | `operation:assign` | `{"add": ["10"], "remove": ["15"]}` |

The permission must exist for the role/user, otherwise `404`. Adding an
existing ID is a no-op.

## Bootstrap refresh

### `POST /admin/sync`

Re-reads every glob in `bootstrap.files` and applies the result (see
[Bootstrap YAML](bootstrap.md)). Requires `resource:write`,
`operation:write`, `role:write`, `role:assign` and `operation:assign`.

```json
{"synced-files": 3}
```
