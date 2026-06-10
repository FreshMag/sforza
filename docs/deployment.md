# Deployment

## Docker image

CI publishes a distroless, statically linked image to GitHub Container
Registry on every push to the default branch and on `v*` tags:

```bash
docker pull ghcr.io/freshmag/sforza:master
```

The image expects its configuration at `/etc/sforza/sforza.yaml` (override
with `SFORZA_CONFIG`). A typical run mounts the config and bootstrap files
and injects secrets through the environment:

```bash
docker run -p 8080:8080 \
  -v ./sforza.yaml:/etc/sforza/sforza.yaml:ro \
  -v ./bootstrap:/etc/sforza/bootstrap:ro \
  -e SHARED_DSN='host=db user=sforza password=... dbname=sforza_shared' \
  -e TENANT_A_DSN='host=db user=sforza password=... dbname=sforza_tenant_a' \
  ghcr.io/freshmag/sforza:master
```

`${VAR}` references inside the YAML are expanded from the container
environment — see [Configuration](configuration.md).

## docker-compose

The repository ships a complete local stack
([docker-compose.yml](https://github.com/FreshMag/sforza/blob/master/docker-compose.yml)):
a Postgres instance initialized with one database for the shared store and
one per tenant, plus Sforza built from source.

```bash
docker compose up --build
curl localhost:8080/healthz
```

## Production checklist

- [ ] `auth.enabled: true` with a reachable OIDC issuer (the realm must be
      up when Sforza boots, since discovery happens at startup).
- [ ] Set `auth.audience` so tokens minted for other services are
      rejected.
- [ ] PostgreSQL or MySQL for the shared and tenant databases; SQLite
      and the JSON store are for development and single-node use.
- [ ] One database per tenant, ideally with distinct credentials.
- [ ] `bootstrap.admin-sub` set to the real administrator subject from
      your identity provider.
- [ ] Bootstrap YAML files mounted read-only.
- [ ] `/healthz` wired to your orchestrator's liveness probe.

## CI/CD

The [CI workflow](https://github.com/FreshMag/sforza/blob/master/.github/workflows/ci.yml):

1. builds the project;
2. runs `go vet` and the full test suite with the race detector and
   coverage;
3. builds the Docker image (with layer caching);
4. pushes it to GHCR — tagged by branch, semver tag and commit SHA — on
   non-PR events.

This documentation site is built with MkDocs and deployed to GitHub Pages
by a separate workflow on every push to the default branch.
