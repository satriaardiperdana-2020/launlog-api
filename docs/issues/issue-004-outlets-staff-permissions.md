# ISSUE-004: Outlets, Staff, and Permissions

## Goal

Allow administrators to manage outlets, staff assignments, and explicit staff permissions within one business.

## Scope

In scope: tenant-scoped outlet profiles and activation, staff account creation/profile/activation, active outlet assignment, permission catalog, staff action grant replacement, last active owner protection, and immutable audit events. Role changes and customer/order operations are out of scope.

## API/database changes

Adds protected `GET/POST /outlets`, `GET/PUT /outlets/{id}`, `GET/POST /staff`, `GET/PUT /staff/{id}`, `PUT /staff/{id}/outlets`, `GET /permissions`, and `PUT /staff/{id}/permissions`. Migration 000009 seeds the permission catalog. All account/assignment/grant writes and their audit rows share one transaction.

## Acceptance criteria

Only active ADMIN principals can manage these resources. Request bodies cannot set business, actor, or role. Staff grants and outlet assignments cannot target the acting admin; assigned outlets must be active and in the same business. Business-row locking protects the last active ADMIN invariant against concurrent account changes. Inactive businesses, actors, and staff with no active outlet are denied. ADMIN bypasses action grants only, while every query remains business-scoped.

## Test cases

Admin success, staff forbidden paths, self-escalation attempts, cross-business assignment rejection, concurrent last-owner changes, inactive outlet assignment/deactivation races, grant/revoke audit events, account audit events, and tenant isolation.

## Branch name

`feature/issue-004-outlets-staff-permissions`

## Definition of done

OpenAPI, services, sqlc queries, tests, audit events, and permission matrix are reviewed and committed.
