# ISSUE-003: Authentication and Sessions

## Goal

Provide secure login, access-token validation, rotating refresh sessions, logout, and current-user context for ADMIN and LAUNDRY_STAFF.

## Scope

In scope: bcrypt, explicit JWT algorithm validation, short-lived access tokens, refresh rotation/revocation, active-user checks, and consistent auth errors. Out of scope: staff administration and permission management UI/API.

## API/database changes

Defines `/auth/login`, `/auth/refresh`, `/auth/logout`, and `/auth/me`; uses the ownership and refresh-token tables from ISSUE-002.

## Acceptance criteria

Credentials never grant caller-supplied authority; invalid, expired, revoked, inactive, or cross-business sessions are rejected; refresh reuse is detected and contained.

## Test cases

Valid login, wrong password, token expiry, algorithm confusion, revoked/replayed refresh token, inactive user, logout, and cross-business access tests.

## Branch name

`feature/issue-003-authentication-and-sessions`

## Definition of done

Contract, handlers, middleware, database queries, integration tests, and security review are complete.
