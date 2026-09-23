# ISSUE-006: Services and Service Types

## Goal

Manage business-scoped laundry services with exact rupiah prices and supported measurement units.

## Scope

In scope: CRUD, activation, soft deletion, pricing, duration, and KILOGRAM/PIECE/METER/SQUARE_METER validation. Out of scope: order line creation.

## API/database changes

Uses `services`; adds protected service endpoints, OpenAPI unit enums, and scoped sqlc queries.

## Acceptance criteria

Prices are whole rupiah integers, units are constrained, deleted services remain usable for historical references only, and staff permissions control writes.

## Test cases

Unit validation, zero/positive price policy, duplicate names by unit, activation, soft deletion, and cross-business access.

## Branch name

`feature/issue-006-services`

## Definition of done

Contract, service/repository implementation, permissions, tests, and audit events are committed.
