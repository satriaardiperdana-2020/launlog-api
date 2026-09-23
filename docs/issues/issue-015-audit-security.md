# ISSUE-015: Audit and Security Hardening

## Goal

Make sensitive actions auditable and close authentication, authorization, input, and operational security gaps.

## Scope

In scope: audit emission, permission review, session replay containment, rate limiting, secure headers/logging, input limits, and security tests. Out of scope: unrelated feature redesign.

## API/database changes

Uses `audit_logs`; may add security middleware, indexes, and narrowly scoped session metadata migrations.

## Acceptance criteria

Sensitive mutations emit immutable audit records without secrets, replayed refresh sessions are contained, auth endpoints resist enumeration/rate abuse, and tenant/outlet checks are enforced consistently.

## Test cases

Audit immutability/emission, replay, enumeration timing mitigation, rate limits, malformed JWTs, permission escalation, and secret-log scans.

## Branch name

`feature/issue-015-audit-security`

## Definition of done

Security review findings are resolved or explicitly accepted, with automated regression tests.
