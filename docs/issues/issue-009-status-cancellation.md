# ISSUE-009: Status History and Cancellations

## Goal

Apply the order lifecycle and retain cancellable-order history safely.

## Scope

In scope: `RECEIVED → PROCESSING → READY_FOR_PICKUP → COMPLETED`, cancellation from any of those three active states, append-only history reads, completion timestamps, mandatory cancellation reason/actor/time, and the existing `deleted_at` soft-delete convention. Paid, partially paid, and refunded orders cannot be cancelled until a refund policy is approved. Out of scope: refund execution and physical deletion.

## API/database changes

Uses existing `order_status_history` and cancellation fields on `orders`; no migration is needed. Adds permission-protected status transition, cancellation, and history endpoints. Each mutation locks the tenant/outlet-scoped order row and commits order update, history, and audit together.

## Acceptance criteria

Only the explicit transition matrix succeeds. Invalid and terminal transitions fail; cancellation of any non-UNPAID order fails with a refund-policy conflict. Cancelled orders remain reportable and retain all order, item, payment, status, and audit history; cancellation uses only the existing soft-delete fields.

## Test cases

Each valid and invalid transition, terminal transition, concurrent update, unpaid/paid cancellation policy, cancellation metadata, history ordering, permissions, active-outlet checks, and tenant isolation.

## Branch name

`feature/issue-009-status-cancellation`

## Definition of done

State-matrix/service tests, PostgreSQL transaction and concurrency tests, OpenAPI contract, and atomic audit coverage are complete.
