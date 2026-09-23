# ISSUE-009: Status History and Cancellations

## Goal

Apply the order lifecycle and retain cancellable-order history safely.

## Scope

In scope: allowed status transitions, history insertion, completion timestamps, cancellation reason/actor, and soft deletion. Out of scope: refund execution.

## API/database changes

Uses `order_status_history` and cancellation fields on `orders`; adds protected transition and cancellation endpoints.

## Acceptance criteria

Every transition and cancellation is atomic, invalid or terminal transitions fail, cancelled orders remain reportable, and cancellation reason is mandatory.

## Test cases

Each valid transition, invalid transition, concurrent update, cancellation metadata, history ordering, permissions, and tenant isolation.

## Branch name

`feature/issue-009-status-cancellation`

## Definition of done

State-machine/service tests, PostgreSQL transaction tests, contract, and audit coverage are complete.
