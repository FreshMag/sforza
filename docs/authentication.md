# Authentication

Sforza authenticates requests with OIDC/OAuth2 bearer tokens. Keycloak is
the primary target, but any compliant issuer exposing OIDC discovery and a
JWKS endpoint works.

## Request requirements

With authentication enabled, every `/api/v1` request must carry:

```http
Authorization: Bearer <JWT>
X-User-Sub: <sub claim of the JWT>
X-Tenant-ID: <tenant>
```

Validation steps:

1. The JWT signature is verified against the issuer's JWKS (fetched via
   OIDC discovery at startup, refreshed automatically).
2. Issuer, expiry and — when configured — audience are enforced.
3. The `X-User-Sub` header must be present and equal to the token's `sub`
   claim; any mismatch is rejected with `401`.
4. The user is **lazily provisioned**: if no user with that `sub` exists
   in the shared database, it is created on the spot.

## Configuration

```yaml
auth:
  enabled: true
  issuer: https://keycloak.example.com/realms/myrealm
  audience: sforza        # optional — empty skips the audience check
```

!!! tip "Keycloak"
    The issuer is the realm URL, e.g.
    `https://keycloak.example.com/realms/myrealm`. Sforza resolves
    `<issuer>/.well-known/openid-configuration` at startup, so the realm
    must be reachable when the service boots.

## Development / test mode

Authentication can be disabled for local development and automated tests:

```yaml
auth:
  enabled: false
  default-sub: test-user
```

In this mode no token is required. The caller identity is taken from the
`X-User-Sub` header when present, falling back to `default-sub` — which
makes it easy to exercise the API as different users:

```bash
curl -H 'X-Tenant-ID: tenant-a' -H 'X-User-Sub: alice' \
     localhost:8080/api/v1/me/operations
```

!!! danger "Never disable authentication in production"
    Authentication defaults to **enabled**; disabling it requires an
    explicit `enabled: false` and the service logs a prominent warning at
    startup.
