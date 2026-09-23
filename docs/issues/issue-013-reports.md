# ISSUE-013: Reports

## Goal

Provide reconciled income, expense, profit/loss, order, cancellation, and customer reports.

## Scope

In scope: date/outlet filters, pagination or export contract, report reconciliation, and authorization. Out of scope: subscription analytics and advanced visualization.

## API/database changes

Adds protected report endpoints and aggregate queries; no denormalized report table without a documented retention strategy.

## Acceptance criteria

Income uses confirmed net payments by payment date, expenses use expense date, cancellation counts include soft-deleted cancellations, and report totals reconcile with dashboard totals.

## Test cases

Known-fixture reconciliation, time-range boundaries, refunds/voids, cancellation retention, outlet scope, and role permissions.

## Branch name

`feature/issue-013-reports`

## Definition of done

Reconciliation integration tests, performance review, OpenAPI, and audit requirements are complete.
