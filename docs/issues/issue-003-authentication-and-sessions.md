# ISSUE-003: Authentication and Sessions

## Goal

Provide secure login, access-token validation, rotating refresh sessions, logout, and current-user context for ADMIN and LAUNDRY_STAFF.

## Scope

In scope: bcrypt, explicit JWT algorithm validation, short-lived access tokens, session-family rotation/revocation, active business/user/outlet checks, bounded login/refresh abuse controls, and consistent auth errors. Out of scope: staff administration and permission management UI/API.

## API/database changes

Defines `/auth/login`, `/auth/refresh`, `/auth/logout`, and `/auth/me`. Migration `000008` adds stable `session_families`, explicitly backfills one family per legacy refresh token, and preserves every token hash. A legacy token's prior ancestry cannot be reconstructed.

## Acceptance criteria

Credentials never grant caller-supplied authority; invalid, expired, revoked, inactive, or cross-business sessions are rejected; refresh reuse is detected and commits family revocation before 401. Refresh and logout serialize on the same family row. The access middleware checks token and family revocation. Login/refresh bodies are limited to 4096 bytes, rate keys are bounded to 4096 per process, and only the direct TCP peer is trusted for IP extraction.

## Test cases

Valid login, wrong password and missing-account indistinguishability, token expiry, algorithm confusion, concurrent refresh/replay, refresh/logout race, inactive business/user/outlet, cross-business access, request limits, and bcrypt byte-boundary tests.

## Session race and delivery semantics

Each successful refresh prepares and signs its response before committing. A response is sent only after commit. If the commit succeeds but the response is lost, the client cannot recover the new opaque refresh token; it must log in again. If a concurrent second use of the old token commits replay revocation, the first successful response may already be in flight but its family is then unusable. This is intentional replay containment.

## Review finding resolution

- Refresh replay: consumed hashes remain indexed; replay commits family revocation before 401. Concurrent refresh tests verify descendants lose access.
- Login abuse/body limits: direct-peer rate keys are bounded and fail closed; all auth bodies have a 4096-byte cap, including streamed requests.
- Public JWT placeholder: the example secret is blank and the old published placeholder is rejected at startup.
- Account timing: unknown/inactive-business accounts perform a cost-12 dummy bcrypt check; valid users also compare bcrypt before inactive-user rejection. Credential failures share one 401 body.
- Logout/refresh race: both lock the stable family row; logout accepts a consumed bearer from the same family when refresh wins first.
- Inactive business/outlet authorization: active-business queries and active-outlet assignments are checked at login, refresh, and access validation; staff requires an active outlet.
- Bcrypt byte limits: new/change password policy is 8–128 Unicode characters and at most 72 UTF-8 bytes; login keeps existing shorter passwords valid and rejects overlong byte strings without account disclosure.

## Branch name

`feature/issue-003-authentication-and-sessions`

## Definition of done

Contract, handlers, middleware, database queries, integration tests, and security review are complete.
