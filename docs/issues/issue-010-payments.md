# ISSUE-010: Payments

## Goal

Record confirmed cash, BCA transfer, and QRIS receipts, maintain order balances, and reconcile payment status safely.

## Scope

In scope: confirmed receipt entry, partial settlement, payment history, balance calculation, idempotent recording, correction voids with reasons, payment-state recalculation, and order-level transaction locking. Out of scope: refunds, automatic bank/QRIS verification, and provider integration. The existing `payment_refunds` table remains unused until refund policy is approved.

## API/database changes

Uses the existing `payments` table and its `CASH`, `BCA_TRANSFER`, and `QRIS` method constraint; no separate payment-method configuration table exists. Adds migration 000013 for nullable idempotency key/hash on legacy-safe existing rows, plus payment receipt/list/void endpoints. `payment_refunds` is not exposed or written. There is no initial-payment field in the approved ISSUE-008 order contract, so order creation continues to start unpaid and payment is recorded separately.

## Acceptance criteria

Confirmed payment amounts cannot exceed the order total; cash tender above the balance is returned as change while only the applied amount is stored as payment revenue. Non-cash overpayment fails. Order payment status and receipt, history, and audit records commit atomically. Payment and cancellation mutations serialize on the parent order.

## Test cases

Partial payments, settlement, cash change, non-cash overpayment, idempotent replay and payload conflict, void/reconciliation, concurrent payment and cancellation, permissions, and cross-business access. No refund case is implemented until approved.

## Branch name

`feature/issue-010-payments`

## Definition of done

Transactional PostgreSQL concurrency tests, OpenAPI, payment-method/schema reconciliation, audit events, and balance reconciliation are complete.
