# ISSUE-004: Outlets, Staff, and Permissions

## Goal

Allow administrators to manage outlets, staff assignments, and explicit staff permissions within one business.

## Scope

In scope: outlet/staff CRUD, activation, outlet assignment, permission catalog, grant/revoke rules, and authorization middleware use. Out of scope: customer and order operations.

## API/database changes

Adds protected outlet, staff, assignment, and permission endpoints; may add permission seeds and narrowly scoped indexes.

## Acceptance criteria

Only an ADMIN can administer staff; staff cannot grant themselves privileges; every query enforces business and outlet scope; inactive actors/outlets are denied.

## Test cases

Admin success, staff forbidden paths, cross-business assignment rejection, activation changes, permission grant/revoke, and outlet-scope checks.

## Branch name

`feature/issue-004-outlets-staff-permissions`

## Definition of done

OpenAPI, services, sqlc queries, tests, audit events, and permission matrix are reviewed and committed.
