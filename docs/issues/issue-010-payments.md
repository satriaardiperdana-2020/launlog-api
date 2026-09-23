# ISSUE-010: Payments

## Goal

Record cash, BCA transfer, and QRIS payments with partial-payment, void, and refund-safe accounting.

## Scope

In scope: payment recording/confirmation/voiding, refund records, payment-state recalculation, and transaction locking. Out of scope: automatic bank or QRIS provider reconciliation.

## API/database changes

Uses `payments` and `payment_refunds`; adds protected payment and refund endpoints with payment-date reporting fields.

## Acceptance criteria

Confirmed net payments cannot exceed the order total, refunds cannot exceed their payment, order payment status is atomically derived, and income uses confirmation time.

## Test cases

Partial payments, overpayment, void/refund limits, concurrent attempts, status reconciliation, permissions, and cross-business access.

## Branch name

`feature/issue-010-payments`

## Definition of done

Transactional PostgreSQL tests, OpenAPI, audit events, and reconciliation tests are complete.
