# ISSUE-005: Customers

## Goal

Manage business-scoped customers and expose safe customer lookup and order-history access.

## Scope

In scope: create, update, detail, search, soft deletion policy, pagination, and customer order history. Out of scope: deposits and loyalty packages.

## API/database changes

Uses `customers`; adds protected customer endpoints and sqlc queries with business filters.

## Acceptance criteria

Customers are isolated by business, phone uniqueness respects soft deletion, lists paginate, and order history cannot reveal another business's data.

## Test cases

Create/update/search, duplicate phone, soft delete, pagination, authorization, and cross-business isolation.

## Branch name

`feature/issue-005-customers`

## Definition of done

Contract, service behavior, repository queries, permissions, tests, and audit logging are complete.
