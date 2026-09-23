# Launlog Database Design

## Design principles

- Entity identifiers use PostgreSQL `BIGSERIAL` primary keys and all corresponding references use `BIGINT`. Join and counter tables use natural composite primary keys.
- Every tenant-owned row carries `business_id`; outlet-owned rows also carry `outlet_id`.
- Composite foreign keys such as `(business_id, outlet_id)` and `(business_id, order_id)` prevent a row from referencing an entity owned by another business.
- Rupiah values use `BIGINT`, never floating point or PostgreSQL `MONEY`.
- Service quantities use `NUMERIC(12,3)`. Piece-based quantities must be whole numbers.
- Timestamps use `TIMESTAMPTZ`. The application and PostgreSQL connection use `Asia/Jakarta` as the business timezone.
- Physical deletion is intentionally restricted by foreign keys. Master records are disabled or soft-deleted, and cancelled orders are retained.
- Database constraints protect row-level invariants. Cross-row totals and workflow operations are performed in service-layer transactions with the relevant order locked.

The composite keys prevent cross-business references, but they do not replace authorization. Every repository query must still filter by the authenticated `business_id` and permitted `outlet_id` values.

## Ownership and access

### `businesses`

The root tenant record. It stores the business name, contact information, active state, and the required `Asia/Jakarta` timezone. All tenant-owned data ultimately references this table.

### `outlets`

A business location. Each outlet belongs to one business, and its code is unique within that business. The `(business_id, id)` unique key is the target for tenant-safe composite foreign keys.

### `users`

An authenticated staff account owned by one business. `role` is constrained to `ADMIN` or `LAUNDRY_STAFF`. Email comparison is case-insensitive and globally unique so login does not require a separate business identifier. Users are deactivated rather than physically deleted.

### `user_outlets`

Assigns a user to one or more outlets. Both composite foreign keys include `business_id`, making it impossible to assign a user from one business to another business's outlet.

### `permissions`

A global catalog of stable action codes seeded by migration `000009_permission_catalog`. Codes are action-scoped (for example `ORDERS_CREATE` or `REPORTS_READ`) and are not tenant identifiers.

### `user_permissions`

Grants an action code to a staff user and records the granting user. Both the recipient and grantor must belong to the same business. Only LAUNDRY_STAFF receive explicit action grants; ADMIN bypasses those grants but all resource queries still require the authenticated business scope.

### `refresh_tokens`

Stores only hashes of refresh tokens, never raw tokens. A token belongs to a user, business, and stable session family, has a required expiry, and may be consumed by rotation. Consumed hashes remain stored to detect replay. The active-token index supports session validation and cleanup.

### `session_families`

Groups all rotations of one login session under a stable key. Refresh and logout lock the family row before changing session state. Revoking a family invalidates all of its access and refresh tokens. Migration `000008` explicitly backfills one family per existing refresh token because earlier rotations did not record ancestry; it preserves all token hashes and business records.

## Master data

### `customers`

A business-wide customer record shared by its outlets. Names are indexed for search. Phone numbers are optional and deliberately non-unique. Migration `000010_customer_phone_nonunique` removes the earlier partial unique index without rewriting customer rows. Its down migration recreates that index and will intentionally fail if duplicate active numbers have been added. Orders reference customers with `(business_id, customer_id)`; customer deactivation sets `deleted_at` and preserves those historical foreign-key references.

### `services`

A business-shared laundry service: every outlet in its owning business can use it. It deliberately has no `outlet_id`; introducing outlet-specific services or pricing would conflict with the current ownership model and needs an approved future design. The existing `services.unit` column is constrained to `KILOGRAM`, `PIECE`, `METER`, or `SQUARE_METER`; price is an exact rupiah `BIGINT` (nonnegative, including zero); estimated duration is a nonnegative integer in minutes. Migration `000011_service_description` adds an optional description without rewriting existing data or changing unit codes/checks. Services can be disabled or soft-deleted while historical order items continue to reference them and retain their original name, unit, and price snapshots. The unique active-row index is per `(business_id, lower(name), unit)` for non-deleted rows.

### `perfumes`

A business-owned perfume catalog. Perfume names are unique among non-deleted records in one business. `PERFUMES_READ` and `PERFUMES_WRITE` gate staff reads and writes. Perfumes can be disabled or soft-deleted without changing historical order data. Order selection is optional; an unselected perfume is represented by NULL foreign-key and snapshot values, never a synthetic master record.

## Orders

### `orders`

The order header belongs to one business, outlet, and customer. Invoice numbers are allocated atomically per outlet and Asia/Jakarta date using `invoice_counters`, formatted `<outlet-code>-<YYYYMMDD>-<sequence padded to at least six digits>`. New orders start in `RECEIVED` with `UNPAID`; ISSUE-008 creates no payment record or initial payment. Status values follow `RECEIVED`, `PROCESSING`, `READY_FOR_PICKUP`, `COMPLETED`, or `CANCELLED`; payment state also allows `PARTIALLY_PAID`, `PAID`, or `REFUNDED` for later issues.

Migration `000012_order_idempotency` adds nullable request key and SHA-256 request hash columns with a unique `(business_id, outlet_id, idempotency_key)` index. Creation serializes concurrent requests for the same key, returns the existing order for the same decoded payload, and rejects a changed payload with HTTP 409. Invoice allocation, order/items, initial status history, and audit log commit in one transaction.

Cancellation is the order soft-delete operation. A cancelled row must have `cancelled_at`, `cancelled_by`, a non-empty reason, and `deleted_at`. Non-cancelled rows cannot carry cancellation metadata. Completed orders require `completed_at`.

### `order_items`

An order line references its source service and optional perfume, while also storing immutable service name, unit, unit price, and perfume-name snapshots. No perfume selection is represented by NULL in both `perfume_id` and `perfume_name_snapshot`; a synthetic perfume record is not used. At most one perfume is selected for the whole order and that selection and its snapshot are applied consistently to its items. Later catalog edits, deactivation, or soft deletion cannot rewrite historical snapshots. Quantity uses `NUMERIC(12,3)` with up to nine integer and three fractional digits; piece quantities must be integral. Each line total is `round(quantity * unit_price_amount_snapshot)` in PostgreSQL numeric arithmetic, which rounds positive half-rupiah values away from zero. The order total is the sum of individually rounded line totals, all stored as whole-rupiah `BIGINT`.

### `order_status_history`

An append-oriented record of every order status transition, including actor and timestamp. Its check constraint permits only the documented forward workflow (`RECEIVED → PROCESSING → READY_FOR_PICKUP → COMPLETED`) and cancellation from `RECEIVED`, `PROCESSING`, or `READY_FOR_PICKUP`. The application locks the order row, applies the state-transition matrix, and updates current status, history, and audit event in one transaction. Cancellation is restricted to orders whose payment status is `UNPAID` until a refund policy is approved; even the existence of `payment_refunds` does not authorize refund behavior.

### `invoice_counters`

Maintains the next invoice sequence per business, outlet, and date. Invoice generation uses an `INSERT ... ON CONFLICT DO UPDATE ... RETURNING` counter upsert so PostgreSQL serializes allocations for a counter key; it creates the order in the same transaction so failures roll back the allocated sequence.

## Payments

### `payments`

Records positive rupiah payments against an order. Multiple confirmed rows provide partial-payment support. The existing `method` CHECK constraint is the canonical method catalog: `CASH`, `BCA_TRANSFER`, and `QRIS`; the current UI-facing label for `BCA_TRANSFER` is “BCA Transfer”. There is no independent payment-method configuration table to reconcile or seed. The application records receipts as `CONFIRMED`; schema lifecycle states also retain `PENDING` and `VOIDED` for existing/future compatibility. Confirmed-payment timestamps are the source for income reporting. For cash, the input tender may exceed the outstanding balance, but `payments.amount` stores only the amount applied to the order; excess is returned as change and is never order revenue.

Recording or voiding a payment must lock the parent order, calculate the confirmed amount, and update `orders.payment_status` in the same transaction. Cancellation locks that same order row and accepts only `UNPAID` orders, so payment and cancellation cannot both commit against stale state. A bounded idempotency key and request hash are added by migration `000013_payment_idempotency`; same key/payload returns the existing receipt and a payload mismatch fails. The order total minus confirmed payment amounts is the outstanding balance. This transactional calculation prevents aggregate values from becoming stale and avoids an unsafe cross-row `CHECK` constraint.

### `payment_refunds`

The actual earlier schema reserves this table for one or more positive refunds against a specific payment, with actor, reason, and timestamp. ISSUE-010 does not insert, expose, or calculate refunds because refund policy is not approved. Its presence alone does not authorize refunds or paid-order cancellation.

## Expenses

### `expense_categories`

A business-wide expense classification such as utilities or supplies. Category names are unique within a business and categories can be disabled while existing expenses remain intact. `EXPENSES_READ` and `EXPENSES_WRITE` protect category and expense operations; ADMIN bypasses these grants but never the authenticated business scope.

### `expenses`

An outlet expense with an exact positive rupiah amount, category, description, occurrence time (`expense_at`), optional receipt reference, and creating user. Composite foreign keys ensure the outlet, category, and actor all belong to the same business. Expense history stays attached to the original outlet. Reads and writes require a currently active outlet and, for staff, a current assignment to that outlet. Inactive categories cannot be selected for a new or updated expense.

Expense reporting filters by `expense_at`, never `created_at`. Inclusive local date bounds are converted to timestamp boundaries in `Asia/Jakarta`; omitted bounds default to today's local date. This timezone selection is explicit in SQL and is independent of the connection session timezone. Income is calculated from confirmed `payments`; expenses cannot create manual income entries.

Migration `000014_expense_versions` adds a positive `version BIGINT NOT NULL DEFAULT 1` to existing records, preserving their data. Expense updates require the client's current ETag (`If-Match: "<version>"`), lock the expense row, compare the version, increment it, and append audit data in the same transaction. A stale version returns 409 instead of overwriting another update. Audit payloads use an allowlist and replace free-text description and receipt reference with `[REDACTED]`; failed auditing rolls back the expense mutation.

## Settings and history

### `receipt_templates`

An outlet-owned receipt layout containing header, footer, WhatsApp message text, and supported paper width. Template names are unique per outlet, and a partial unique index allows at most one active default template per outlet.

### `audit_logs`

An immutable business audit stream. It records an optional outlet and actor, action, polymorphic entity identity, before/after JSON objects, client address, user agent, and occurrence time. Tenant-safe foreign keys apply to the outlet and actor; `entity_id` is intentionally polymorphic and therefore has no single-table foreign key. A trigger rejects updates and deletes.

## Migration order

1. `000001_ownership_and_access`
2. `000002_master_data`
3. `000003_orders`
4. `000004_payments`
5. `000005_expenses`
6. `000006_receipts_and_audit`
7. `000007_ownership_hardening`
8. `000008_session_families`
9. `000009_permission_catalog`
10. `000010_customer_phone_nonunique`
11. `000011_service_description`
12. `000012_order_idempotency`
13. `000013_payment_idempotency`
14. `000014_expense_versions`

Each migration has matching `.up.sql` and `.down.sql` files. Rollbacks must run in reverse order because later domains reference earlier ownership and order tables.

## Operational migration policy

Migrations are executed with the pinned golang-migrate CLI through `make migrate-up`, `make migrate-down`, and `make migrate-version`. `make migrate-verify` performs an up/down/up cycle and is run against an isolated PostgreSQL 16 service in CI. It must never be pointed at production as a smoke test.

Applied migrations are immutable: correct a deployed schema with a later migration rather than editing an existing migration file. Generated sqlc output must be regenerated after every migration or query change.

## Reconciliation note

The repository's actual migration history contains master-data, order, payment, expense, receipt, and audit tables in `000002` through `000006`; later issues progressively implement their API behavior. Their earlier presence alone did not mark those issues complete. In particular, the pre-existing `payment_refunds` table remains reserved and unused because refund policy is not approved. Migration `000013_payment_idempotency` adds only nullable request-key/hash metadata and a partial unique index; it preserves all existing payment rows and leaves their idempotency fields NULL.

## Deliberately excluded

The schema contains no customer deposits, quota packages, subscription plans, subscription billing, or related balance tables.
