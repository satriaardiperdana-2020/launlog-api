# ISSUE-008: Orders and Invoice Numbers

## Goal

Create tenant-safe orders with atomic invoice allocation and immutable service snapshots.

## Scope

In scope: order creation/detail/listing, invoice counter locking, decimal quantities, line totals, and service/perfume snapshots. Out of scope: status transitions and payments.

## API/database changes

Uses `orders`, `order_items`, and `invoice_counters`; adds protected order endpoints and transactional sqlc queries.

## Acceptance criteria

Invoice numbers are unique per outlet/date policy, all items and totals are server-calculated in one transaction, and no request can select another business's customer/service/perfume.

## Test cases

Concurrent invoice allocation, decimal and PIECE quantity validation, snapshot immutability, total calculation, rollback, and cross-business attempts.

## Branch name

`feature/issue-008-orders-invoices`

## Definition of done

Transaction tests against PostgreSQL, contract, permission checks, and audit events are complete.
