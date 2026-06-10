# Getting Started

## Run in development mode

Development mode uses SQLite and disables authentication, so nothing else
is required:

```bash
git clone https://github.com/francesco/sforza
cd sforza
mkdir -p data
go run ./cmd/sforza -config configs/sforza.dev.yaml
```

The dev config declares two tenants (`tenant-a`, `tenant-b`), loads the
example bootstrap file from `bootstrap/`, and creates the administrator
`admin-user-sub`.

## First requests

With authentication disabled, the `X-User-Sub` header selects the caller:

```bash
# Effective operations for john in tenant-a
curl -H 'X-Tenant-ID: tenant-a' -H 'X-User-Sub: john' \
     localhost:8080/api/v1/me/operations
```

```json
[
  {"operation": "invoice:approve", "scope": "FULL"},
  {"operation": "invoice:read", "scope": "FULL"},
  {"operation": "product:read", "scope": "FULL"},
  {"operation": "product:write", "scope": "RESTRICTED"}
]
```

```bash
# Record IDs for the RESTRICTED operation
curl -H 'X-Tenant-ID: tenant-a' -H 'X-User-Sub: john' \
     'localhost:8080/api/v1/me/record-ids?operations=product:write'
```

```json
{"product:write": ["10", "15", "42"]}
```

Administration uses the same API as the bootstrap administrator:

```bash
# Create a role and grant it a permission
curl -X POST -H 'X-Tenant-ID: tenant-a' -H 'X-User-Sub: admin-user-sub' \
     -d '{"name":"auditor"}' localhost:8080/api/v1/roles

curl -X PUT -H 'X-Tenant-ID: tenant-a' -H 'X-User-Sub: admin-user-sub' \
     -d '{"scope":"FULL"}' \
     localhost:8080/api/v1/roles/auditor/permissions/invoice:read

# Assign it
curl -X POST -H 'X-Tenant-ID: tenant-a' -H 'X-User-Sub: admin-user-sub' \
     localhost:8080/api/v1/roles/auditor/assignments/alice
```

## Run the full stack

`docker compose up --build` starts Postgres (one database for the shared
store, one per tenant) and Sforza:

```bash
docker compose up --build
curl localhost:8080/healthz
```

See [Deployment](deployment.md) for production images and
[Authentication](authentication.md) for enabling OIDC.
