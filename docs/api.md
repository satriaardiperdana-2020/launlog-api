# Launlog API

The source of truth is [api/openapi.yaml](../api/openapi.yaml). The current server base path is `/`; versioning is deferred until a versioned public API is introduced.

## Foundation endpoints

- `GET /livez` is public process liveness and always returns `200 {"status":"ok"}` while Echo is running.
- `GET /readyz` is public database readiness. It returns `200 {"status":"ok"}` when PostgreSQL responds within `READINESS_TIMEOUT`, otherwise `503 {"status":"unavailable"}`.
- `GET /health` is a compatibility alias for `/readyz`.

## Existing access endpoints

- `POST /auth/login` and `POST /auth/refresh` are public.
- `POST /auth/logout` and `GET /auth/me` require `Authorization: Bearer <access-token>`.
- Auth request bodies are capped at 4096 bytes. One per-process limiter allows 10 requests per minute for each direct TCP peer and route; forwarding headers are not trusted. A full 4096-key limiter rejects new keys until entries expire.
- Credential failures use the same 401 response. A valid, cost-12 dummy bcrypt hash is checked when the email has no active account.
- At password creation or change, require 8–128 Unicode characters and at most 72 UTF-8 bytes. Login checks existing passwords without imposing that character minimum; bcrypt's byte limit still applies. OpenAPI `minLength`/`maxLength` count characters, not UTF-8 bytes.
- Rotating refresh tokens retain consumed hashes to detect replay. Replay revokes the session family. If a committed rotation response is lost, the client must log in again because the raw replacement token is not stored.

## Owner-managed outlets and staff (ISSUE-004)

The authenticated `ADMIN` role manages its own business through `GET/POST /outlets`, `GET/PUT /outlets/{outletId}`, `GET/POST /staff`, and `GET/PUT /staff/{userId}`. It can replace a laundry staff account's assignments with `PUT /staff/{userId}/outlets`, list the action catalog at `GET /permissions`, and replace a staff member's action grants with `PUT /staff/{userId}/permissions`. These routes return `403` to `LAUNDRY_STAFF`; IDs and business scope are always derived/validated against the authenticated business, never accepted as authority from JSON.

Staff creation always creates `LAUNDRY_STAFF`, requires at least one active outlet in the same business, and uses the authentication password-creation policy. Role changes and admin self-assignment/grants are not supported. An active staff member cannot be left without an active outlet, and the last active administrator cannot be deactivated. Outlet deactivation is rejected while it would strand active staff. Administrative writes, including account, assignment, permission, and outlet changes, insert an immutable audit event in the same transaction; audit records never contain password hashes or plaintext passwords.

`ADMIN` bypasses action grants only. All management queries remain scoped to the authenticated `business_id`; the `ADMIN` role cannot address another business by changing a path or body ID. `LAUNDRY_STAFF` authorization on operational endpoints must use a current session, active outlet assignment, and the endpoint's explicit action grant.

## Customers (ISSUE-005)

`GET/POST /customers`, `GET/PUT/DELETE /customers/{customerId}`, and `GET /customers/{customerId}/orders` use the authenticated business scope. List/search uses `page`, `pageSize`, and optional case-insensitive `q` over name and phone. Name is required; phone and address are optional and phone values may repeat. On full `PUT`, omitted or null optional fields are cleared. `DELETE` sets `customers.deleted_at`; it does not remove a customer row or break its order foreign keys. Deactivated customers are omitted from active list/detail results, while their retained order history remains accessible.

Staff needs `CUSTOMERS_READ` to list/detail customers, `CUSTOMERS_WRITE` to create/update/deactivate, and both `CUSTOMERS_READ` and `ORDERS_READ` for customer order history. Staff history is restricted to their current active outlet assignments. Admin history uses the same business filter and bypasses grants only. History is read from the existing `orders` table, paginated by `created_at` and ID, and includes cancelled orders because cancellation is a retained soft-delete state.

## Contract rules

- Add all endpoint changes to OpenAPI before generating Echo types with `make generate`.
- IDs and rupiah values are signed 64-bit JSON integers. Decimal quantities are strings with up to three fractional digits.
- Date-times are RFC 3339 values with an explicit offset; business-facing values use Asia/Jakarta.
- Do not accept actor, user, business, or outlet authority from request JSON. Derive it from authenticated context.
- List endpoints added by later issues must use the shared `page` and `pageSize` conventions.
