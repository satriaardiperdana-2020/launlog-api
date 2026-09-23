# Launlog Architecture

## Repositories

- launlog-api: Go backend and API contract
- launlog-app: Vue frontend, created after backend acceptance
- launlog-be: historical reference only

## Backend layers

- handlers: OpenAPI/Echo HTTP adapters
- service: business rules, calculations, and transaction orchestration
- repository: PostgreSQL connection and sqlc access
- middleware: JWT, active-user, permission, and outlet checks
- security: password hashing, access tokens, refresh tokens
- domain: statuses, constants, and business errors

## Request flow

Client -> Echo middleware -> OpenAPI handler -> service layer -> repository/sqlc -> PostgreSQL

The frontend must never be trusted to enforce permissions or calculate authoritative financial totals.

## Foundation lifecycle

Configuration is loaded once at startup. The process parses the PostgreSQL URL, applies pool limits and the Asia/Jakarta session timezone, then pings PostgreSQL within the configured connection timeout. Only then does Echo begin accepting requests.

`/livez` reports only process liveness. `/readyz` and `/health` use a short, bounded PostgreSQL ping and report readiness. On `SIGINT` or `SIGTERM`, `http.Server.Shutdown` stops new requests, waits up to `SHUTDOWN_TIMEOUT` for in-flight requests, and then closes the pool.

The current authentication handlers are preserved as compatible implementation work. New business behavior must be added through the service and repository layers defined above rather than directly from request JSON to database operations.

## Authentication

The backend validates the access token, active account, business membership, outlet assignment, and required permission. Refresh tokens are stored and revocable.

## Transaction boundaries

Order creation, order cancellation, payment recording, and owner provisioning run inside database transactions.

## External/device responsibilities

The backend provides receipt data, QR identifiers, and WhatsApp templates. The client device handles camera scanning, thermal printer pairing, and opening WhatsApp.
