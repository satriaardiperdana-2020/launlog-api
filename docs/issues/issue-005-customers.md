# ISSUE-005: Customers

## Goal

Manage business-scoped customers and expose safe customer lookup and order-history access.

## Scope

In scope: create, update, detail, search, soft deletion policy, pagination, and customer order history. Customer name is required; phone and address are optional. Phone numbers are not unique because the product requirements do not mandate uniqueness. Deactivation is a soft deletion and must preserve order foreign keys. Out of scope: deposits and loyalty packages.

## API/database changes

Uses existing `customers` and `orders` tables. Adds protected customer CRUD/search/deactivation endpoints and a paginated `/customers/{customerId}/orders` query over the actual `orders` schema. Migration 000010 drops the existing business/phone unique index without modifying customer/order rows; rolling it back can fail if duplicate active phone numbers have since been entered.

## Acceptance criteria

Customers are isolated by business, duplicate phone numbers are accepted, lists paginate, and order history cannot reveal another business's data or orders outside a staff member's active outlet assignments. Customer deactivation preserves historical order references; cancelled order rows remain included in history.

## Test cases

Create/update/search, repeated phone values, soft deactivation with preserved order references, paginated history from the existing order schema (including cancelled orders), permission denial, staff outlet filtering, and cross-business isolation.

## Branch name

`feature/issue-005-customers`

## Definition of done

Contract, service behavior, repository queries, permissions, tests, and audit logging are complete.

## Order-history dependency

Closed by ISSUE-008: customer history reads actual persisted `orders` rows created through `POST /orders`, remains paginated, and includes retained cancelled orders while respecting current staff outlet assignments.
