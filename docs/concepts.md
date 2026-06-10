# SFBAC Concepts

SFBAC (Scoped Functions-Based Access Control) extends classic RBAC with a
**visibility scope** on every grant. Authorization answers two questions
instead of one: *may the user perform this operation?* and *on which
records?*

## Resources

A **resource** is a logical domain entity — `product`, `customer`,
`invoice`. Resources are globally defined, stored in the shared database,
and act as containers for operations.

## Operations

An **operation** is an action on a resource, named `resource:action`:

```text
product:read
product:write
invoice:approve
```

Operations are globally defined. When an operation is registered, its
parent resource is derived from the name prefix and created automatically
if missing.

## Scopes

Every grant pairs an operation with one of three scopes:

| Scope | Meaning |
|---|---|
| `FULL` | Access to **all** records of the operation. |
| `EMPTY` | Access to **no** records. |
| `RESTRICTED` | Access to an explicit set of record IDs. |

!!! note "Record IDs are strings"
    IDs are stored and returned as strings so integers and UUIDs both
    work. YAML integers are coerced: `ids: [10, 15]` becomes
    `["10", "15"]`.

## Roles

A **role** is a named set of `(operation, scope)` pairs, defined per
tenant:

```text
manager
├── product:read  -> FULL
├── product:write -> RESTRICTED [10, 15, 42]
└── invoice:read  -> FULL
```

Users may hold any number of roles.

## User overrides

Permissions can also be attached directly to a user. A user-level
permission **always** overrides role grants for the same operation — in
both directions:

```text
role grant:     product:read -> RESTRICTED [1, 2, 3]
user override:  product:read -> FULL
effective:      product:read -> FULL
```

```text
role grant:     invoice:read -> FULL
user override:  invoice:read -> EMPTY
effective:      invoice:read -> EMPTY        (override revokes access)
```

## Resolution rules

For each operation, within the active tenant:

1. **User override first.** If the user has a direct permission for the
   operation, that is the effective scope — roles are not consulted.
2. **Roles merge by widest scope.** Otherwise all the user's roles are
   combined and `FULL > RESTRICTED > EMPTY`. When several roles grant
   `RESTRICTED`, their record ID sets are **unioned**.
3. **Deny-by-default.** An operation with no assignment anywhere is simply
   absent from the effective set: not allowed.

Restricted IDs follow the same precedence: when a user-level `RESTRICTED`
permission is in effect, only the user's own ID set applies — role ID sets
are ignored entirely.

### Worked example

```text
roles of pippo:
  support:  product:read -> RESTRICTED [1, 2]
  sales:    product:read -> RESTRICTED [2, 3]
            invoice:read -> EMPTY
  auditor:  invoice:read -> FULL

user overrides:
  invoice:approve -> FULL
```

Effective permissions:

| Operation | Scope | Why |
|---|---|---|
| `product:read` | `RESTRICTED` `[1, 2, 3]` | union of the two role ID sets |
| `invoice:read` | `FULL` | FULL (auditor) beats EMPTY (sales) |
| `invoice:approve` | `FULL` | user override |
| anything else | — | deny-by-default |
