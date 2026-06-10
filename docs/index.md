# Sforza

Sforza is a centralized authorization service implementing **SFBAC**
(Scoped Functions-Based Access Control), a variation of RBAC where every
permission is an `(operation, scope)` pair instead of a bare grant.

It is designed for microservice ecosystems:

- **Multi-tenant** — one shared database for global entities, one physical
  database per tenant for roles and permissions.
- **OIDC / OAuth2 authentication** — Keycloak-first, but any compliant
  issuer works; a configurable disabled mode supports development and
  testing.
- **Fine-grained visibility** — every grant carries a scope: `FULL`,
  `EMPTY`, or `RESTRICTED` to an explicit set of record IDs.
- **User overrides** — direct user permissions always beat role grants.
- **Deny-by-default** — no assignment means no access; there are no
  implicit permissions.
- **YAML bootstrap** — each microservice contributes a human-readable file
  declaring its resources, operations, roles and users; synchronization is
  additive and idempotent, at startup and on demand.
- **Self-authorizing** — Sforza's own administrative APIs are protected by
  the same model ([meta authorization](meta-authorization.md)).
- **Docker-native** — distroless image, environment-driven configuration,
  published to GHCR by CI.

## How a permission check works

A consuming service asks Sforza two questions on behalf of a user:

```bash
# 1. What can this user do?
GET /api/v1/me/operations
# [{"operation":"product:read","scope":"FULL"},
#  {"operation":"invoice:read","scope":"RESTRICTED"}]

# 2. Which records can they touch, for RESTRICTED operations?
GET /api/v1/me/record-ids?operations=invoice:read
# {"invoice:read":["10","20"]}
```

The service then enforces the result locally — typically `FULL` means no
filter, `RESTRICTED` becomes a `WHERE id IN (...)` clause, and a missing
operation means the request is rejected.

## Where to go next

- [Getting Started](getting-started.md) — run Sforza in two minutes.
- [SFBAC Concepts](concepts.md) — resources, operations, scopes, roles,
  overrides, and the exact resolution rules.
- [REST API](api.md) — the full endpoint reference.
