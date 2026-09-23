# ISSUE-012: Dashboard

## Goal

Provide outlet-scoped operational totals for income, expenses, unpaid orders, and current workload.

## Scope

In scope: date-range defaults in Asia/Jakarta, confirmed-payment income, expense totals, and authorized outlet aggregation. Out of scope: advanced charts and forecasting.

## API/database changes

Adds protected dashboard endpoints and aggregate sqlc queries; no new business tables are expected.

## Acceptance criteria

Dashboard totals use the same payment/expense definitions as reports, honor outlet permissions, and handle empty ranges deterministically.

## Test cases

Timezone boundary, confirmed versus pending/voided payment, refund, outlet isolation, empty data, and permission tests.

## Branch name

`feature/issue-012-dashboard`

## Definition of done

Aggregate queries are reconciled against fixture transactions and documented in OpenAPI.
