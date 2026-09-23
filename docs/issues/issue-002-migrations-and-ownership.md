# ISSUE-002: Migrations and Ownership

## Goal

Establish tenant-safe PostgreSQL ownership, outlet, user, permission, and refresh-session schema.

## Scope

In scope: versioned up/down migrations, BIGINT identifiers, business/outlet isolation, roles, assignments, permission grants, session lookup indexes, ownership annotations, and PostgreSQL integration coverage. Out of scope: HTTP management endpoints and business workflows.

## API/database changes

The historical `000001` migration already adds `businesses`, `outlets`, `users`, `user_outlets`, `permissions`, `user_permissions`, and `refresh_tokens`. This issue adds `000007_ownership_hardening` without rewriting those deployed migrations: it adds the full tenant session lookup index and documents tenant keys. No public resource API is added.

`services` is business-shared in the actual schema (`business_id`, no `outlet_id`). This agrees with the current documentation; outlet-specific services or pricing would be a separate design decision, not an implicit migration change.

## Acceptance criteria

Composite foreign keys reject cross-business assignments and sessions, role values are constrained, token hashes—not raw tokens—are stored, ownership comments identify the tenant model, and migrations run cleanly on an empty PostgreSQL 16 database. Existing business records and sessions are preserved; the additive migration needs no data backfill.

## Test cases

Migration up/down/up, cross-business user-outlet and refresh-token rejection, service unit validation, business-shared service scope, ownership comments, unique email/outlet constraints, and refresh-token expiry/revocation constraints.

## Branch name

`feature/issue-002-migrations-and-ownership`

## Definition of done

Migrations, sqlc schema generation, and isolated migration tests are committed with no edits to previously deployed migrations.
