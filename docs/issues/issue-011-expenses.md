# ISSUE-011: Expenses

## Goal

Manage expense categories and outlet expenses using exact rupiah amounts.

## Scope

In scope: category management, expense create/list/detail, date filtering, permissions, and soft operational controls. Out of scope: supplier accounts and inventory.

## API/database changes

Uses `expense_categories` and `expenses`; adds protected endpoints and scoped sqlc queries.

## Acceptance criteria

Amounts are positive whole rupiah, category/outlet/actor belong to one business, inactive categories cannot be used for new expenses, and listings paginate.

## Test cases

Category lifecycle, expense validation, date filters, outlet scope, permission denial, and cross-business references.

## Branch name

`feature/issue-011-expenses`

## Definition of done

Contract, service/repository code, tests, and audit coverage are complete.
