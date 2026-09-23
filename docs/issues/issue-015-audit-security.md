# ISSUE-015: Audit and Security Hardening

## Goal

Make sensitive actions auditable and close authentication, authorization, input, and operational security gaps.

## Scope

In scope: audit emission, permission review, session replay containment, rate limiting, secure headers/logging, input limits, and security tests. Out of scope: unrelated feature redesign.

## API/database changes

Uses `audit_logs`, exact-origin CORS configuration, security/cache headers, pinned dependency updates, a vulnerability scan, and a least-privilege PostgreSQL runtime role. Authentication/session lifecycle events use the existing audit table; no raw token, password, or hash is written there.

## Security review evidence and fixes

- `internal/security/token.go` pins HS256 and issuer/audience/expiry; `internal/middleware/auth.go` reloads active user/session/active outlets and current permissions on every authenticated request. Refresh/logout serialize on a stable family row, and replay revocation commits before 401. Regression tests cover these ISSUE-003 invariants.
- The read-only dependency review found pgx/v5 v5.5.5 before the v5.9.0 fix range for GO-2026-4771 and Echo v4.15.1 before the v4.15.3 fix for GO-2026-6293. Both are upgraded; CI runs pinned govulncheck v1.8.0.
- Customer audit values previously contained names, phone numbers, and addresses. Audit entries retain the entity ID/action but redact personal fields.
- Login, refresh rotation, logout revocation, and refresh replay containment write token-free session-family audit events in their state transaction.
- CORS was disabled (deny by default), with no wildcard. It remains disabled unless an exact `CORS_ALLOWED_ORIGINS` allowlist is configured. API security headers and no-store auth responses are regression-tested.
- Production PostgreSQL startup rejects superuser, role/database administration, schema-creation, and audit mutation/deletion privileges; TLS must verify the database hostname. CI uses separate migration/runtime login roles.
- Read-only query review found business IDs consistently included in tenant entity queries; outlet operations additionally enforce active outlets and current staff assignment. ADMIN bypasses action grants only. Error responses return generic 500s and health readiness hides database errors. No request-body or credential logging was found.

## Acceptance criteria

Sensitive mutations emit immutable audit records without secrets, replayed refresh sessions are contained, auth endpoints resist enumeration/rate abuse, and tenant/outlet checks are enforced consistently.

## Test cases

Audit immutability/emission, replay, enumeration timing mitigation, rate limits, malformed JWTs, permission escalation, and secret-log scans.

## Branch name

`feature/issue-015-audit-security`

## Definition of done

Security review findings are resolved or explicitly accepted, with automated regression tests.
