# ISSUE-007: Perfumes

## Goal

Manage business-scoped perfume catalog entries for future order selection.

## Scope

In scope: CRUD, activation, soft deletion, search, and permissions. Out of scope: inventory and supplier tracking.

## API/database changes

Uses `perfumes`; adds protected catalog endpoints and business-scoped sqlc queries.

## Acceptance criteria

Names are unique among active records in a business, historical order snapshots are unaffected by later edits, and unauthorized users cannot modify catalog data.

## Test cases

Create/update/list, duplicate names, deactivation/deletion, permission denial, and cross-business isolation.

## Branch name

`feature/issue-007-perfumes`

## Definition of done

OpenAPI, handlers/services, queries, tests, and audit events are complete.
