# ISSUE-002: Backend Foundation Implementation Plan

## Goal

Establish the runnable Launlog API foundation: a Go 1.21 module, an Echo HTTP server, typed configuration, PostgreSQL connectivity, a public health endpoint, orderly shutdown, and automated tests. This issue creates no business-domain schema or endpoints.

## Requirements alignment

- The backend remains the API authority and uses the layering in [architecture.md](../architecture.md): HTTP adapters, services, repositories, and domain types.
- `GET /health` is a public endpoint required by [api.md](../api.md).
- PostgreSQL is the application database. Database migrations, ownership tables, sqlc queries, OpenAPI resources, authentication, customers, orders, and reports are explicitly deferred to later issues.
- `go.mod` currently exists as an untracked file and declares `go 1.27.1`. The implementation must replace that declaration with `go 1.21` before dependencies are resolved; it must not rely on features newer than Go 1.21.

## Scope

### In scope

- Initialize and maintain the `github.com/satriaardiperdana-2020/launlog-api` module for Go 1.21.
- Create the executable API entry point and the internal package layout.
- Load and validate runtime configuration from environment variables.
- Build a PostgreSQL pool, verify it during startup, and close it on shutdown.
- Serve `GET /health` with a JSON response.
- Handle operating-system termination signals gracefully.
- Add unit and integration-test foundations and CI-friendly commands.

### Out of scope

- Migrations and all domain tables.
- sqlc configuration or generated query code.
- OpenAPI specification, oapi-codegen output, Swagger UI, and feature handlers.
- Authentication, authorization, customers, services, orders, payments, expenses, dashboards, and reports.

## Implementation plan

### 1. Go module and toolchain

1. Set the module path in `go.mod` to `github.com/satriaardiperdana-2020/launlog-api` and set its `go` directive to `1.21`.
2. Use a Go 1.21 toolchain in local development and CI. Do not add a `toolchain` directive that requires a newer Go release.
3. Run `go mod tidy` after dependency installation and commit both `go.mod` and `go.sum`.
4. Add a `Makefile` (or documented equivalent) with reproducible targets for `run`, `test`, `test-integration`, `vet`, and `build`.

### 2. Dependencies

Select maintained releases whose own Go module requirements allow Go 1.21; pin their resolved versions in `go.mod`.

| Need | Dependency | Use |
| --- | --- | --- |
| HTTP framework | `github.com/labstack/echo/v4` | Router, middleware, request lifecycle, and server integration |
| PostgreSQL driver/pool | `github.com/jackc/pgx/v5` and `pgxpool` | Context-aware PostgreSQL connections and pooling |
| Environment configuration | `github.com/caarlos0/env/v11` | Typed environment-variable decoding and required-value validation |
| Structured logging | `log/slog` (standard library) | JSON or text logs without an additional logging dependency |
| Assertions | `github.com/stretchr/testify` | Readable unit and handler tests |
| Integration-test helpers | `github.com/testcontainers/testcontainers-go` | Ephemeral PostgreSQL only for the integration test suite |

`sqlc`, `oapi-codegen`, and migration tooling are deferred. When introduced, they should be version-pinned as development tools and checked in CI against Go 1.21.

### 3. Folder structure

Create only the packages needed for the foundation, keeping later business modules additive.

```text
.
├── cmd/api/main.go                 # process composition and signal handling
├── internal/app/app.go             # application construction and lifecycle
├── internal/config/config.go       # typed configuration and validation
├── internal/http/health.go          # health handler and response type
├── internal/http/router.go          # Echo instance, middleware, routes
├── internal/repository/postgres.go  # pgxpool creation, ping, close
├── internal/testutil/               # test-only configuration/HTTP helpers
├── migrations/                      # empty placeholder; populated by the migration issue
├── Makefile
├── go.mod
└── go.sum
```

Keep `cmd/api` thin: it reads configuration, constructs `app.App`, runs it, and maps startup failures to a non-zero exit. HTTP code must not access environment variables directly; repository code must not know about Echo.

### 4. Configuration strategy

Define a single immutable `config.Config` loaded once at startup. Use environment variables, with local development values stored in an uncommitted `.env` file and an `.env.example` listing names only.

| Variable | Required | Example / default | Purpose |
| --- | --- | --- | --- |
| `APP_ENV` | No | `development` | Environment label for logging and behavior |
| `HTTP_HOST` | No | `0.0.0.0` | Bind address |
| `HTTP_PORT` | No | `8080` | Bind port |
| `HTTP_READ_TIMEOUT` | No | `10s` | Request-header/read timeout |
| `HTTP_WRITE_TIMEOUT` | No | `15s` | Response-write timeout |
| `HTTP_IDLE_TIMEOUT` | No | `60s` | Keep-alive idle timeout |
| `SHUTDOWN_TIMEOUT` | No | `10s` | Maximum graceful-shutdown window |
| `DATABASE_URL` | Yes | PostgreSQL connection URI | Database credentials and connection target |
| `DATABASE_MAX_CONNS` | No | `10` | Maximum pgx pool size |
| `DATABASE_MIN_CONNS` | No | `0` | Minimum pgx pool size |
| `DATABASE_CONNECT_TIMEOUT` | No | `5s` | Bounded initial connection and ping time |

Validation must reject an empty `DATABASE_URL`, invalid durations/ports, and non-positive limits before the server starts. Never log `DATABASE_URL` or any secret-bearing configuration value.

### 5. PostgreSQL connection approach

1. Parse `DATABASE_URL` with pgx pool configuration.
2. Apply pool limits and an application name such as `launlog-api`.
3. Create the pool with a startup context bounded by `DATABASE_CONNECT_TIMEOUT`, then call `Ping` before accepting HTTP traffic.
4. If parsing, creation, or the initial ping fails, log a sanitized error and exit non-zero; do not run a partially functional server.
5. Expose only a small repository interface for liveness/readiness checks so handlers do not couple to `pgxpool.Pool`.
6. On shutdown, stop accepting traffic first and then close the pool after in-flight requests have drained.

### 6. Echo startup and routing

1. Construct Echo in `internal/http/router.go`.
2. Add request ID, panic recovery, and structured request logging middleware. Configure error handling to return a consistent JSON error shape without exposing internal errors.
3. Register `GET /health` before any versioned API groups. Future application APIs will live under `/api/v1`, as specified in `docs/api.md`.
4. Configure an `http.Server` with the configured address and timeouts, using Echo as its handler.
5. Start serving only after configuration validation and PostgreSQL ping succeed.

### 7. Health endpoint contract

`GET /health` is unauthenticated and returns:

```json
{"status":"ok"}
```

It returns HTTP 200 only when the application can reach PostgreSQL through the repository health interface. If PostgreSQL is unavailable, return HTTP 503 with the standard JSON error response and log the underlying failure server-side. The response must not disclose connection details, migration state, users, or version metadata.

### 8. Graceful shutdown

1. Listen for `SIGINT` and `SIGTERM` using `signal.NotifyContext`.
2. On a signal, stop accepting new connections with `http.Server.Shutdown` using `SHUTDOWN_TIMEOUT`.
3. Allow active requests to complete during that window. If it expires, log the timeout and force server closure.
4. Close the PostgreSQL pool exactly once after server shutdown returns.
5. Return a non-zero exit only for startup failures or an unrecoverable server error; a normal signal-driven shutdown exits successfully.

### 9. Testing strategy

| Level | Coverage | Execution |
| --- | --- | --- |
| Unit | Configuration defaults and validation, router registration, health response mapping, and shutdown orchestration through interfaces/fakes | `go test ./...` |
| Handler | HTTP 200 response when the health dependency succeeds; HTTP 503 and safe error body when it fails; method-not-allowed behavior | `go test ./internal/http/...` |
| Repository integration | pgx pool can connect and ping a disposable PostgreSQL instance; invalid/unreachable configuration fails startup cleanly | `go test -tags=integration ./...` |
| Quality gate | Formatting, compilation, static analysis, race checks where practical | `gofmt -w`, `go build ./...`, `go vet ./...`, `go test -race ./...` |

Integration tests must obtain the database URL from a test container or explicit test-only environment variable; they must never point at a developer or production database. Keep the default unit-test suite independent of Docker and PostgreSQL.

## Acceptance criteria

1. `go.mod` declares the Launlog module and `go 1.21`; a clean Go 1.21 environment completes `go mod tidy`, `go build ./...`, `go test ./...`, and `go vet ./...`.
2. Configuration is loaded once from documented environment variables, applies the stated defaults, rejects invalid values, and never writes secrets to logs.
3. The process does not listen for HTTP traffic until PostgreSQL configuration is valid and the initial database ping succeeds.
4. A running application responds to unauthenticated `GET /health` with HTTP 200 and `{"status":"ok"}` when PostgreSQL is reachable.
5. `GET /health` returns HTTP 503 with a sanitized JSON error response when the health dependency cannot reach PostgreSQL.
6. `SIGINT` and `SIGTERM` stop the HTTP server within `SHUTDOWN_TIMEOUT`, drain in-flight requests when possible, and close the PostgreSQL pool without a panic or connection leak.
7. Unit tests cover configuration, health success/failure, and graceful shutdown; PostgreSQL integration tests cover a real pool connection and ping.
8. No customer, authentication, order, payment, report, migration, sqlc, OpenAPI, Swagger, or frontend feature code is included in this issue.

## Definition of done

- The acceptance criteria and required checks in [development-workflow.md](../development-workflow.md) pass.
- The implementation is reviewed from `feature/issue-002-backend-foundation`.
- Environment examples contain no credentials, and local secret files are ignored.
