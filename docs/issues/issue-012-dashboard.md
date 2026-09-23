# ISSUE-012: Dashboard

## Goal

Provide the selected outlet profile, today's confirmed payment receipts, and today's expenses.

## Scope

In scope: one authenticated active outlet, its profile, business-timezone calendar date, confirmed payment count/amount, and expense count/amount for that date. In the current schema an outlet inherits the required business timezone (`Asia/Jakarta`); day boundaries are computed from that stored timezone. Out of scope: unpaid-order totals, workload/status charts, advanced charting/forecasting, net revenue, refunds, and persistent materialized summaries.

## API/database changes

Adds `GET /outlets/{outletId}/dashboard` protected by `REPORTS_READ` for staff. One sqlc query aggregates `payments` and `expenses` independently before joining them to the outlet profile. Payment totals use only `status=CONFIRMED` rows and `confirmed_at`; expense totals use `expense_at`. Order totals, pending/voided payments, and `created_at` are not income/expense sources. Refund rows are not applied because refund policy/behavior is not approved. No new migration, business table, or persistent summary table is required.

## Acceptance criteria

Return only rows inside the authenticated business and selected active outlet; staff must have a current assignment and `REPORTS_READ`, while ADMIN bypasses the grant but not tenant ownership. Start/end timestamps correspond to the outlet's local day, inclusive start/exclusive end. Totals/counts are exact whole-rupiah `BIGINT`/`int64` values with deterministic zeros for empty days. Independent child aggregation prevents payment/expense join multiplication.

## Test cases

Timezone boundary (`confirmed_at`/`expense_at` exactly at start and end), confirmed versus pending/voided receipts, no use of order totals or `created_at`, receipt/expense aggregate independence, refund non-application until approved, cross-outlet and cross-business isolation, empty data, and permission tests.

## Branch name

`feature/issue-012-dashboard`

## Definition of done

Aggregate queries are reconciled against fixture transactions and documented in OpenAPI.
