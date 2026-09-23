# ISSUE-002: Migrations and Ownership

## Goal

Establish tenant-safe PostgreSQL ownership, outlet, user, permission, and refresh-session schema.

## Scope

In scope: versioned up/down migrations, BIGINT identifiers, business/outlet isolation, roles, assignments, permission grants, and indexes. Out of scope: HTTP management endpoints and business workflows.

## API/database changes

Adds `businesses`, `outlets`, `users`, `user_outlets`, `permissions`, `user_permissions`, and `refresh_tokens`; no public resource API.

## Acceptance criteria

Composite foreign keys reject cross-business assignments, role values are constrained, token hashes—not raw tokens—are stored, and migrations run cleanly on an empty PostgreSQL 16 database.

## Test cases

Migration up/down/up, cross-business foreign-key rejection, unique email/outlet constraints, and refresh-token expiry/revocation constraints.

## Branch name

`feature/issue-002-migrations-and-ownership`

## Definition of done

Migrations, sqlc schema generation, and isolated migration tests are committed with no edits to previously deployed migrations.
