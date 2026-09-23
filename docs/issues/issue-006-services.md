# ISSUE-006: Services and Service Types

## Goal

Manage business-scoped laundry services with exact rupiah prices and supported measurement units.

## Scope

In scope: business-shared CRUD, search, unit and active-state filters, activation/deactivation, soft deletion, descriptions, exact pricing, duration, and existing KILOGRAM/PIECE/METER/SQUARE_METER validation. Out of scope: order line creation and outlet-specific prices.

## API/database changes

Uses `services.unit` as the measurement type; there is no `service_type` column. Adds nullable `services.description` in migration 000011 without changing existing rows or unit codes/check constraints. Adds `GET/POST /services`, `GET/PUT/DELETE /services/{serviceId}`, OpenAPI unit enums, business-scoped sqlc queries, and transactional service audit events.

## Acceptance criteria

Prices are nonnegative whole rupiah integers (zero remains valid under the existing CHECK), duration is nonnegative minutes, unit codes remain case-sensitive, deleted services remain retained for historical references, and staff permissions control reads and writes. All service queries filter on the authenticated business; ADMIN bypasses grants only. Updating or deleting a service does not mutate order-item snapshots.

## Test cases

Unit validation against existing database codes, zero/positive and negative prices, negative duration, duplicate names by unit, search/filter/pagination, activation/deactivation, soft deletion, cross-business access, staff permission denial, audit persistence, and historical order snapshots.

## Branch name

`feature/issue-006-services`

## Definition of done

Contract, service/repository implementation, permissions, tests, and audit events are committed.
