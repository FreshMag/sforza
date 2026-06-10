# Development

## Building and testing

```bash
go build ./...
go vet ./...
go test -race -coverpkg=./internal/... ./...
```

Tests run entirely against SQLite — no external services needed. The
storage layer is identical for Postgres (GORM with driver selection per
DSN), and CI runs the same suite with the race detector.

The suite covers:

- **Resolution** — user overrides (including `EMPTY` silencing `FULL`),
  multi-role widest-scope merging, restricted ID unions, deny-by-default.
- **Multi-tenancy** — role and permission isolation at both service and
  API level, tenant-scoped meta permissions.
- **Meta authorization** — denial without grants, `FULL`-only enforcement,
  delegation of individual meta operations.
- **Authentication** — OIDC validation against an in-process fake
  provider: expired, forged-signature, wrong-audience and mismatched
  `X-User-Sub` tokens; disabled-mode behavior; lazy provisioning.
- **Bootstrap** — idempotent re-sync, scope updates, unknown tenant and
  invalid scope rejection, admin bootstrap.
- **API surface** — every admin endpoint, error mapping, header
  validation.

## Project layout

```
cmd/sforza          entrypoint (config, bootstrap, graceful shutdown)
internal/config     YAML + env-expansion configuration
internal/model      domain types: scopes, meta operation catalog
internal/store      GORM models, shared + per-tenant database handles
internal/service    resolution, administration, bootstrap synchronization
internal/auth       OIDC and development authenticators
internal/api        chi router, middleware, handlers
internal/testutil   shared test fixtures
bootstrap/          example microservice bootstrap file
configs/            example service configurations
deploy/             docker-compose support files
docs/               this documentation site (MkDocs)
```

## Architectural notes

- **Layering** — handlers only parse/serialize HTTP; all behavior lives in
  `internal/service`, which operates on `gorm.DB` handles passed from the
  store layer. Sentinel errors (`ErrNotFound`, `ErrConflict`,
  `ErrValidation`) map to status codes in one place.
- **Tenant isolation** — a tenant is selected once, in middleware, by
  resolving `X-Tenant-ID` to its dedicated database handle; nothing below
  the middleware ever sees another tenant's data.
- **Idempotency** — migrations, meta bootstrap and YAML sync can all run
  repeatedly; startup order is migrate → meta/admin bootstrap → file sync.
- **Authenticator interface** — `auth.Authenticator` has two
  implementations (OIDC, static); tests inject either.

## Documentation site

```bash
pip install mkdocs-material
mkdocs serve        # live preview at http://127.0.0.1:8000
```

The site deploys to GitHub Pages automatically on pushes to the default
branch.
