# Launlog API

Launlog is the Go backend for a multi-outlet laundry POS. It uses Echo, PostgreSQL 16, pgx/v5, sqlc, OpenAPI 3.0, and oapi-codegen. The application timezone is fixed to Asia/Jakarta.

## Prerequisites

- Go 1.27.1
- Docker Compose with PostgreSQL 16, or an equivalent local PostgreSQL 16 instance
- GNU Make

The repository pins its generation and migration tools in the [Makefile](Makefile): sqlc `v1.31.1`, oapi-codegen `v2.7.0`, and golang-migrate `v4.18.1`.

| Component | Selected version |
| --- | --- |
| Go | 1.27.1 |
| Echo | v4.15.1 |
| pgx/v5 | v5.5.5 |
| PostgreSQL | 16-alpine |
| sqlc | v1.31.1 |
| oapi-codegen | v2.7.0 |
| golang-migrate | v4.18.1 |

## Local setup

1. Copy `.env.example` to `.env.development`.
2. Set a unique `JWT_SIGNING_SECRET` of at least 32 bytes. Do not commit this file.
3. Start PostgreSQL: `make compose-up`.
4. Apply migrations: `DATABASE_URL='...' make migrate-up`.
5. Start the API: `make run`.

Configuration loads `.env.<APP_ENV>` first (default `.env.development`) and then `.env`; explicitly exported environment variables take precedence. See `.env.example` for every supported setting.

| Variable | Default | Notes |
| --- | --- | --- |
| `HTTP_HOST`, `HTTP_PORT` | `0.0.0.0`, `8080` | HTTP bind address |
| `HTTP_*_TIMEOUT`, `SHUTDOWN_TIMEOUT` | 10s/15s/60s/10s | Positive durations only |
| `READINESS_TIMEOUT` | `2s` | Bound for `/readyz` database ping |
| `APP_TIMEZONE` | `Asia/Jakarta` | The only accepted business/session timezone |
| `DATABASE_URL` | none | Required; never log or commit it |
| `DATABASE_*_CONNS`, `DATABASE_CONNECT_TIMEOUT` | 10/0/5s | pgx pool and startup ping limits |
| `JWT_SIGNING_SECRET` | none | Required; at least 32 bytes |
| `JWT_ACCESS_TOKEN_TTL`, `JWT_REFRESH_TOKEN_TTL` | 15m/720h | Auth-session limits |

## Operational endpoints

- `GET /livez`: process liveness; does not contact PostgreSQL.
- `GET /readyz`: readiness; returns 503 while PostgreSQL is unavailable.
- `GET /health`: compatibility alias for `/readyz`.

## Development commands

```shell
make tools            # install pinned local generators into ./bin
make generate          # regenerate OpenAPI and sqlc output
make check             # formatting, generated-code drift, tests, vet, build
make migrate-verify    # up, down, and up again; requires DATABASE_URL
TEST_DATABASE_URL='...' make test-integration # PostgreSQL migration and tenant-isolation integration suite
```

`make check` is the local equivalent of the quality CI job. The migration CI job uses an isolated PostgreSQL service and never uses developer or production credentials.
