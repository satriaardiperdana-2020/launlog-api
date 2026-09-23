# ISSUE-007: Perfumes

## Goal

Manage business-scoped perfume catalog entries for future order selection.

## Scope

In scope: business-scoped CRUD, search, activation/deactivation, soft deletion, and permissions. Out of scope: inventory, supplier tracking, and order creation.

## API/database changes

Uses the existing `perfumes` schema with no migration. Adds `GET/POST /perfumes`, `GET/PUT/DELETE /perfumes/{perfumeId}`, OpenAPI schemas, business-scoped sqlc queries, and transactional audit events. The perfume remains optional at order time; an absent selection uses NULL references/snapshots, not a synthetic master row. Order-level selection cardinality and snapshots are defined for ISSUE-008.

## Acceptance criteria

Names are unique among non-deleted records in one business; active state can be changed, soft deletion preserves order foreign keys, historical snapshots are unaffected by edits, and staff access uses PERFUMES_READ/PERFUMES_WRITE. All lookups are tenant-scoped.

## Test cases

Create/update/list, duplicate names, deactivation/deletion, permission denial, and cross-business isolation.

## Branch name

`feature/issue-007-perfumes`

## Definition of done

OpenAPI, handlers/services, queries, tests, and audit events are complete.
