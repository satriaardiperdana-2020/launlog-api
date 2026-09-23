# ISSUE-008: Orders and Invoice Numbers

## Goal

Create tenant-safe orders with atomic invoice allocation and immutable service snapshots.

## Scope

In scope: order creation/detail/listing, invoice counter locking, decimal quantities, line totals, and service/perfume snapshots. A single optional perfume may be selected for the whole order; when none is selected, perfume IDs and snapshots are NULL and no synthetic perfume record is used. The order transaction validates that all lines use that same selected perfume (or none). It snapshots the selected perfume name with the order items. Later perfume edits, deactivation, or deletion must not alter historical snapshots. Out of scope: status transitions and payments.

## API/database changes

Uses `orders`, `order_items`, and `invoice_counters`; adds protected order endpoints and transactional sqlc queries.

## Acceptance criteria

Invoice numbers are unique per outlet/date policy, all items and totals are server-calculated in one transaction, and no request can select another business's customer/service/perfume. At most one perfume is selected across all items in an order; no selection stores NULL in both perfume reference and name snapshot fields. Enforce this across rows in the order creation transaction because current row-level checks alone do not guarantee the cardinality rule. Every order item retains its service snapshots and the chosen perfume name snapshot.

## Test cases

Concurrent invoice allocation, decimal and PIECE quantity validation, snapshot immutability, total calculation, rollback, and cross-business attempts.

## Branch name

`feature/issue-008-orders-invoices`

## Definition of done

Transaction tests against PostgreSQL, contract, permission checks, and audit events are complete.

## Implementation decisions

- Migration `000012_order_idempotency` adds a business/outlet-scoped key and SHA-256 payload hash.
- Invoice numbers are `<outlet-code>-<YYYYMMDD>-<sequence padded to at least six digits>`; allocation uses one counter upsert in the order transaction.
- `NUMERIC(12,3)` quantity allows at most nine integer and three fractional digits. Piece quantities are integral. Each line is multiplied exactly and rounded to whole rupiah using PostgreSQL numeric rounding (half away from zero); the order total sums rounded lines.
- Orders are created `UNPAID`; payment collection remains ISSUE-010. Request payloads cannot set a price.
- One optional perfume is selected for all order items. Absence stores NULL perfume IDs and snapshots. Item snapshots preserve current catalog names/units/prices at creation.
- Customer history from ISSUE-005 is now backed by orders created through this API.
