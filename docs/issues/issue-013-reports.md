# ISSUE-013: Reports

## Goal

Provide reconciled income, expense, profit/loss, order, cancellation, and customer reports.

## Scope

In scope: date/outlet filters, pagination or export contract, report reconciliation, and authorization. Out of scope: subscription analytics and advanced visualization.

## API/database changes

Adds protected report endpoints and aggregate queries; no denormalized report table without a documented retention strategy.

### Metric contract (documented before query implementation)

All report date filters are inclusive outlet-local calendar dates (`fromDate`, `toDate`), converted to a half-open timestamp range in the business timezone (currently constrained to `Asia/Jakarta`). Omitted bounds mean the current local day. Every request is scoped to one active outlet in the authenticated business; staff additionally need a current active-outlet assignment and `REPORTS_READ`. ADMIN bypasses only the grant/assignment requirement, not tenant ownership or outlet activity. Reports are live reads of current rows, not historical snapshots. Where records are editable, a past report can therefore change after an edit; only the event's date basis is historical.

| Report | Date basis and population | Grouping / values | Cancellation and as-of semantics |
| --- | --- | --- | --- |
| Income | `payments.confirmed_at` in range; only `status='CONFIRMED'` | Daily and payment-method totals, count, exact rupiah received | PENDING and VOIDED receipts excluded. A confirmed payment is gross collected income; the reserved refund table is not subtracted because refunds are not an approved workflow. Current payment status controls inclusion. |
| Expenses | `expenses.expense_at` in range; all rows for the outlet | Daily totals, count, exact rupiah expense | No soft-delete convention exists for expenses. Use current row amount/date/category, so edits can change historical periods. `created_at` is never the date basis. |
| Profit/loss | The union of income dates and expense dates from the two populations above | Daily and period income, expenses, and income-minus-expenses; child sources aggregated independently | Expense and income follow their own event dates. This is cash-received less recorded expense, not order-value accrual accounting; gross confirmed receipts are not reduced by unapproved refunds. |
| Orders | `orders.received_at` in range; all orders including retained soft-deleted cancellations | Daily and period order count and recorded order value; current non-cancelled outstanding balance; confirmed collection attached to this received-date cohort; separate cancelled count/value | Order value is `orders.total_amount`, collected is confirmed `payments.amount`, and outstanding is `GREATEST(total_amount - confirmed payments, 0)` only for non-cancelled orders. Canceled orders remain in count/value and are reported separately but do not contribute collectible outstanding. Current order/payment state is used; payment dates do not constrain cohort collection. |
| Cancellations | `orders.cancelled_at` in range; `status='CANCELLED'` | Daily and period cancellation count and canceled recorded order value | Include canceled rows even though `deleted_at` is set; never filter soft-deleted cancellations out. Current cancellation status/timestamp/value are used. No refund amount or paid-cancellation eligibility is inferred. |
| Customers | Customers with at least one order whose `received_at` is in range; canceled orders remain in cohort | Period distinct customer count and deterministic top 10 customers by qualifying order count, then order value descending, normalized name ascending, customer ID ascending | Counts distinct customer IDs, not order rows. Customer name and current order values are live; no as-of customer-name history exists. Ties resolve with stable secondary keys. |

The orders report groups by order-received date while its cohort collection is the confirmed lifetime collection currently attached to those orders; it is not the income report, which groups each payment by `confirmed_at`. All balances and monetary values are integer rupiah. Separate pre-aggregates are required for payments, orders, cancellations, and expenses so joins cannot multiply totals.

### Endpoint contract

`GET /outlets/{outletId}/reports/income`, `/expenses`, `/profit-loss`, `/orders`, `/cancellations`, and `/customers`. All accept inclusive `fromDate` and `toDate` in `YYYY-MM-DD`; with neither bound, both default to today in the outlet timezone; with only one bound, both resolve to that single date. The maximum range is 366 inclusive local dates. Each response has `from_date`, `to_date`, `timezone`, summary totals, and the report-specific daily/method/customer breakdown. These are bounded aggregate responses (daily series and customer ranking capped at 10); no pagination or export job is needed for this contract. No report tables are introduced. Migration 000015 adds supporting indexes for order received-date cohorts and confirmed-payment aggregation by order.

## Acceptance criteria

Income uses confirmed payments by payment date (VOIDED excluded; no unapproved refund netting), expenses use expense date, cancellation counts include soft-deleted cancellations, and income/expense daily totals reconcile with dashboard totals for matching local dates.

## Test cases

Hand-calculated reconciliation of order value, received payments, outstanding balances, expenses, profit/loss, distinct customers, and deterministic top-10 ties; time-range boundaries; void exclusion and refund non-netting; cancellation retention; outlet scope; and role permissions.

## Branch name

`feature/issue-013-reports`

## Definition of done

Reconciliation integration tests, performance review, OpenAPI, and audit requirements are complete.
