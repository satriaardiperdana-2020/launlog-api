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

## Services (ISSUE-006)

`GET/POST /services` and `GET/PUT/DELETE /services/{serviceId}` manage the business-shared catalog. Staff requires `SERVICES_READ` for reads and `SERVICES_WRITE` for writes; administrators bypass action grants but never business ownership. List supports `q` over name/description, exact `unit`, optional `isActive`, and shared pagination. Inactive services remain visible to authorized readers; soft-deleted services do not. `PUT` is a full update and uses `isActive` to deactivate/reactivate; `DELETE` soft-deletes. A service cannot be restored after soft deletion through this API.

The existing `unit` field is the service measurement type: `KILOGRAM`, `PIECE`, `METER`, or `SQUARE_METER`. Codes are case-sensitive and match the database CHECK; there is no second `service_type`. Prices are nonnegative, exact whole-rupiah `int64` values (zero is allowed by the current schema). Duration is a nonnegative `int32` number of minutes, defaulting to zero on create. Service names are unique per business and unit among non-deleted rows. All writes are audited. Changing or deleting a service never rewrites an existing order-item snapshot.

## Perfumes (ISSUE-007)

`GET/POST /perfumes` and `GET/PUT/DELETE /perfumes/{perfumeId}` manage the authenticated business's catalog. Staff needs `PERFUMES_READ` for reads and `PERFUMES_WRITE` for writes; ADMIN bypasses action grants while queries remain business-scoped. List supports `q` across name/description, optional `isActive`, and `page`/`pageSize`. Names are unique per business among non-deleted rows. `PUT` replaces name/description and active state; `DELETE` sets inactive and soft-deletes, retaining the row for existing order foreign keys. All writes are audited.

Perfume selection is optional for orders. No selection means `order_items.perfume_id` and `perfume_name_snapshot` are both NULL; no placeholder perfume row is needed. ISSUE-008 must enforce the agreed maximum of one selected perfume across an order, and snapshot the selected name on order items in the order transaction. Later catalog edits/deactivation/deletion must not change those snapshots.

## Orders and invoices (ISSUE-008)

`POST /orders` requires an `Idempotency-Key` header and creates the order, items, invoice, initial `RECEIVED` history event, and audit event in one transaction. Keys are scoped to business and outlet; same key and decoded payload returns the existing order with `Idempotency-Replayed: true`, while same key with another payload returns `409 IDEMPOTENCY_CONFLICT`. Invoice numbers use `<outlet-code>-<YYYYMMDD>-<sequence padded to at least six digits>` based on the database session's Asia/Jakarta date and an atomic `invoice_counters` upsert, never a row count. Outlet/customer/service/perfume scope derives from the authenticated business and the selected outlet must be active and currently assigned to staff.

The request has one or more service items. It cannot set prices: each active service's current name, unit, and whole-rupiah unit price are copied into an immutable item snapshot. Quantity is a decimal string with at most nine integer and three fractional digits (`NUMERIC(12,3)`); `PIECE` quantity must be integral and every quantity must be positive. Each line amount is calculated independently using PostgreSQL numeric rounding (half-rupiah rounds away from zero); the order total is the sum of the rounded line amounts. A due date is optional and must not precede the server-set receive time. A single optional perfume applies to all lines; without a selection both perfume fields are NULL. New orders start `UNPAID`; payment recording is handled separately by ISSUE-010.

`GET /orders` is paginated and filters by optional outlet and status; staff only sees currently assigned active outlets. `GET /orders/{orderId}` uses the same outlet and business isolation. The existing `GET /customers/{customerId}/orders` now returns order history backed by records created through this API, closing the history dependency from ISSUE-005.

## Contract rules

- Add all endpoint changes to OpenAPI before generating Echo types with `make generate`.
- IDs and rupiah values are signed 64-bit JSON integers. Decimal quantities are strings with up to three fractional digits.
- Date-times are RFC 3339 values with an explicit offset; business-facing values use Asia/Jakarta.
- Do not accept actor, user, business, or outlet authority from request JSON. Derive it from authenticated context.
- List endpoints added by later issues must use the shared `page` and `pageSize` conventions.
