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

## Authentication

The backend validates the access token, active account, business membership, outlet assignment, and required permission. Refresh tokens are stored and revocable.

## Transaction boundaries

Order creation, order cancellation, payment recording, and owner provisioning run inside database transactions.

## External/device responsibilities

The backend provides receipt data, QR identifiers, and WhatsApp templates. The client device handles camera scanning, thermal printer pairing, and opening WhatsApp.
