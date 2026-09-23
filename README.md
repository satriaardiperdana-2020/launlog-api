# Launlog API

Launlog is the Go backend for a multi-outlet laundry POS. It uses Echo, PostgreSQL 16, pgx/v5, sqlc, OpenAPI 3.0, and oapi-codegen. The application timezone is fixed to Asia/Jakarta.

## Prerequisites

- Go 1.27.1
- Docker Compose with PostgreSQL 16, or an equivalent local PostgreSQL 16 instance
- GNU Make
- PostgreSQL 16 client utilities (`psql`, `pg_dump`, `pg_restore`) and `curl` for the full release rehearsal

The repository pins its generation, migration, lint, and vulnerability-scan tools in the [Makefile](Makefile): sqlc `v1.31.1`, oapi-codegen `v2.7.0`, golang-migrate `v4.18.1`, golangci-lint `v2.13.2`, and govulncheck `v1.8.0`.

| Component | Selected version |
| --- | --- |
| Go | 1.27.1 |
| Echo | v4.15.4 |
| pgx/v5 | v5.11.0 |
| PostgreSQL | 16-alpine |
| sqlc | v1.31.1 |
| oapi-codegen | v2.7.0 |
| golang-migrate | v4.18.1 |
| golangci-lint | v2.13.2 |
| Embedded Swagger UI assets | github.com/swaggo/files/v2 v2.0.1 |

## Local setup

1. Copy `.env.example` to `.env.development`.
2. Generate a local key with `openssl rand -hex 32`, put its output in `JWT_SIGNING_SECRET`, and do not commit this file.
3. Start PostgreSQL: `make compose-up`.
4. Apply local migrations: `DATABASE_URL='...' make migrate-up` (the local example intentionally reuses the development database role).
5. Start the API: `make run`.

Configuration loads `.env.<APP_ENV>` first (default `.env.development`) and then `.env`; explicitly exported environment variables take precedence. See `.env.example` for every supported setting.

| Variable | Default | Notes |
| --- | --- | --- |
| `HTTP_HOST`, `HTTP_PORT` | `0.0.0.0`, `8080` | HTTP bind address |
| `HTTP_*_TIMEOUT`, `SHUTDOWN_TIMEOUT` | 10s/15s/60s/10s | Positive durations only |
| `READINESS_TIMEOUT` | `2s` | Bound for `/readyz` database ping |
| `APP_TIMEZONE` | `Asia/Jakarta` | The only accepted business/session timezone |
| `DATABASE_URL` | none | Required; never log or commit it |
| `MIGRATION_DATABASE_URL` | `DATABASE_URL` | Schema-owner/migration role; use a different credential from runtime in production |
| `CORS_ALLOWED_ORIGINS` | empty | Exact comma-separated `http(s)://host[:port]` allowlist; empty disables CORS, wildcards are rejected |
| `DATABASE_*_CONNS`, `DATABASE_CONNECT_TIMEOUT` | 10/0/5s | pgx pool and startup ping limits |
| `JWT_SIGNING_SECRET` | none | Required; at least 32 bytes |
| `JWT_ACCESS_TOKEN_TTL`, `JWT_REFRESH_TOKEN_TTL` | 15m/720h | Auth-session limits |

## Operational endpoints

- `GET /livez`: process liveness; does not contact PostgreSQL.
- `GET /readyz`: readiness; returns 503 while PostgreSQL is unavailable.
- `GET /health`: compatibility alias for `/readyz`.

## Interactive API docs

In development (`APP_ENV=development`, the default), open [http://localhost:8080/swagger](http://localhost:8080/swagger) after starting the API. Swagger UI loads the checked-in OpenAPI contract from `/openapi.yaml`; its assets are served locally, so no CDN access is needed. Use **Try it out** to send requests to this running API. Authenticated operations accept a bearer access token via the Authorize control. The UI and contract routes are disabled outside development.

## Development commands

```shell
make tools            # install pinned local generators into ./bin
make generate          # regenerate OpenAPI and sqlc output
make check             # formatting, generated-code drift, tests, vet, lint, build, govulncheck
make migrate-verify    # up, down, and up again; requires MIGRATION_DATABASE_URL (DATABASE_URL fallback for local dev)
make release-check     # complete release gate; requires separate disposable test/upgrade/restore database URLs
```

`make release-check` refuses to silently skip PostgreSQL-backed suites: provide `TEST_DATABASE_URL`, `TEST_ADMIN_DATABASE_URL`, `TEST_EXPECT_LEAST_PRIVILEGE=true`, `E2E_DATABASE_URL`, `UPGRADE_MIGRATION_DATABASE_URL`, `UPGRADE_ADMIN_DATABASE_URL`, `BACKUP_SOURCE_DATABASE_URL`, and `RESTORE_DATABASE_URL`. These must target isolated disposable databases and use separate runtime, migration, and test-administrator credentials. CI provisions them in ephemeral PostgreSQL 16. See the [release and operations runbook](docs/release-readiness.md).

Production must use a non-superuser `DATABASE_URL` role with no schema-creation or audit-update/delete privileges. The application verifies this and requires `sslmode=verify-full` when `APP_ENV=production`. See [database role setup](docs/database.md#runtime-database-role).

Deployment and rollback procedures are documented in the [release and operations runbook](docs/release-readiness.md).
