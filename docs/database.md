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

A global catalog of permission codes. Codes use uppercase identifiers such as `ORDER_UPDATE`; specific permission rows can be introduced with seeds when authorization features are implemented.

### `user_permissions`

Grants a permission to a staff user and records the granting user. Both the recipient and grantor must belong to the same business. `ADMIN` access remains an application rule; staff receives explicit grants.

### `refresh_tokens`

Stores only hashes of refresh tokens, never raw tokens. A token belongs to a user, business, and stable session family, has a required expiry, and may be consumed by rotation. Consumed hashes remain stored to detect replay. The active-token index supports session validation and cleanup.

### `session_families`

Groups all rotations of one login session under a stable key. Refresh and logout lock the family row before changing session state. Revoking a family invalidates all of its access and refresh tokens. Migration `000008` explicitly backfills one family per existing refresh token because earlier rotations did not record ancestry; it preserves all token hashes and business records.

## Master data

### `customers`

A business-wide customer record shared by its outlets. Names are indexed for search, and non-deleted phone numbers are unique within a business. Orders reference customers with `(business_id, customer_id)`.

### `services`

A business-shared laundry service: every outlet in its owning business can use it. It deliberately has no `outlet_id`; introducing outlet-specific services or pricing would conflict with the current ownership model and needs an approved future design. The existing `services.unit` column is constrained to `KILOGRAM`, `PIECE`, `METER`, or `SQUARE_METER`; price is an exact rupiah `BIGINT`; estimated duration is stored in minutes. Services can be disabled or soft-deleted while historical order items continue to reference them.

### `perfumes`

A business-owned perfume catalog. Perfume names are unique among non-deleted records in one business. Perfumes can be disabled or soft-deleted without changing historical order data.

## Orders

### `orders`

The order header belongs to one business, outlet, and customer. Invoice numbers are unique per outlet. Status values follow `RECEIVED`, `PROCESSING`, `READY_FOR_PICKUP`, `COMPLETED`, or `CANCELLED`; payment state is `UNPAID`, `PARTIALLY_PAID`, `PAID`, or `REFUNDED`.

Cancellation is the order soft-delete operation. A cancelled row must have `cancelled_at`, `cancelled_by`, a non-empty reason, and `deleted_at`. Non-cancelled rows cannot carry cancellation metadata. Completed orders require `completed_at`.

### `order_items`

An order line references its source service and optional perfume, while also storing immutable service name, unit, unit price, and perfume-name snapshots. This prevents later catalog changes from rewriting historical receipts and reports. Quantity uses `NUMERIC(12,3)`, piece quantities must be integral, and the line total must equal the rounded snapshot price multiplied by quantity.

### `order_status_history`

An append-oriented record of every order status transition, including actor and timestamp. Its check constraint permits only the documented forward workflow and cancellation from active states. Order updates and history insertion must occur in one transaction so the current order state and its history remain consistent.

### `invoice_counters`

Maintains the next invoice sequence per business, outlet, and date. Invoice generation must lock the counter row with `SELECT ... FOR UPDATE`, increment it, and create the order in the same transaction.

## Payments

### `payments`

Records positive rupiah payments against an order. Multiple confirmed rows provide partial-payment support. Methods are constrained to `CASH`, `BCA_TRANSFER`, or `QRIS`; lifecycle states are `PENDING`, `CONFIRMED`, and `VOIDED`. Confirmed-payment timestamps are the source for income reporting.

Recording or voiding a payment must lock the order, calculate the net confirmed amount, and update `orders.payment_status` in the same transaction. This transactional calculation prevents aggregate values from becoming stale and avoids an unsafe cross-row `CHECK` constraint.

### `payment_refunds`

Records one or more positive refunds against a specific payment, with actor, reason, and timestamp. The four-column composite foreign key guarantees the payment belongs to the same business, outlet, and order. Refund totals are validated transactionally while locking the payment and order.

## Expenses

### `expense_categories`

A business-wide expense classification such as utilities or supplies. Category names are unique within a business and categories can be disabled while existing expenses remain intact.

### `expenses`

An outlet expense with an exact positive rupiah amount, category, description, occurrence time, optional receipt reference, and creating user. Composite foreign keys ensure the outlet, category, and actor all belong to the same business.

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

Each migration has matching `.up.sql` and `.down.sql` files. Rollbacks must run in reverse order because later domains reference earlier ownership and order tables.

## Operational migration policy

Migrations are executed with the pinned golang-migrate CLI through `make migrate-up`, `make migrate-down`, and `make migrate-version`. `make migrate-verify` performs an up/down/up cycle and is run against an isolated PostgreSQL 16 service in CI. It must never be pointed at production as a smoke test.

Applied migrations are immutable: correct a deployed schema with a later migration rather than editing an existing migration file. Generated sqlc output must be regenerated after every migration or query change.

## Reconciliation note

The repository's actual migration history already contains master-data, order, payment, expense, receipt, and audit tables in `000002` through `000006`, even though their HTTP and service-layer work belongs to later issues. Their presence is schema reservation only; it does not mark those issues complete. In particular, the pre-existing `payment_refunds` table is not changed or exposed by this issue; no refund behavior is approved or implemented here. ISSUE-002 therefore adds only the forward-safe `000007_ownership_hardening` migration: a non-partial session lookup index and ownership comments. It has no data-shape change and therefore no data backfill. Existing businesses, outlets, users, and refresh sessions are neither deleted nor rewritten.

## Deliberately excluded

The schema contains no customer deposits, quota packages, subscription plans, subscription billing, or related balance tables.
