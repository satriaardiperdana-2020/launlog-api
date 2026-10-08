# ISSUE-017: Platform Admin Identity and Audit Foundation

## Goal

Establish exactly one active `PLATFORM_ADMIN` identity for platform operations, with authentication and audit records separate from every business. This issue builds the platform security foundation; tenant management and support operations belong to ISSUE-018.

## Scope

In scope: restricted one-time bootstrap, platform login/refresh/logout/current-identity endpoints, platform session revocation and replay containment, strict platform-versus-tenant credential separation, immutable platform audit, database grants, and regression tests. Out of scope: public platform registration, platform self-service account creation, tenant data access, and business management endpoints.

`users` remains a business-owned table with `business_id` and role `ADMIN` or `LAUNDRY_STAFF`. `PLATFORM_ADMIN` must not be added to that role check, `user_outlets`, tenant permissions, tenant session tables, or business audit actors.

## API and database design

- Add an additive migration after `000017_service_unit_codes`, with matching up/down files. `platform_admins` is a singleton account table: fixed `id = 1`, nonempty normalized email and password hash, `is_active = TRUE`, timestamps, and a unique case-insensitive email. The fixed key and checks prevent a second or inactive platform account. Before first bootstrap there are zero accounts; successful bootstrap establishes the exactly-one-active invariant. Password rotation updates the same row and revokes its sessions; account deletion/deactivation is not an API operation.
- Add `platform_session_families` and `platform_refresh_tokens` with platform-admin foreign keys, unique hashes, expiry/revocation checks, and indexes for active lookup and replay detection. Store refresh hashes only. Keep consumed hashes for replay detection. Refresh and logout lock the family row; replay revokes the family before returning 401.
- Add `platform_audit_logs` with an optional `actor_platform_admin_id` (NULL for initial bootstrap), action, target type/ID, outcome, request ID, occurred time, and bounded, allowlisted metadata. A trigger rejects `UPDATE` and `DELETE`. Do not store passwords, tokens, token hashes, authorization headers, customer data, or arbitrary request/response bodies. Use explicit target references where possible; audit events for bootstrap and session lifecycle are required.
- Add `/platform/auth/login`, `/platform/auth/refresh`, `/platform/auth/logout`, and `/platform/auth/me` to OpenAPI. No `/platform/register` route. Apply the existing auth body limit, direct-peer rate limit, generic credential errors, and `Cache-Control: no-store` semantics to the platform endpoints. Return a platform identity response without tenant `businessId`, `outletIds`, or tenant permissions.
- Define a distinct platform JWT audience (for example `launlog-platform`), explicit token kind, subject and session ID, short expiry, pinned algorithm and issuer. The tenant audience remains `launlog-api`. Prefer a separate platform signing secret configured through the deployment secret manager. Neither parser accepts the other's audience or token kind. Do not infer an identity class from a numeric subject or a role string alone.
- Add platform authentication middleware that reloads the active singleton and session on every protected platform request. Tenant middleware must reject platform tokens before tenant database queries; platform middleware must reject tenant tokens. Platform authorization is scoped only to explicitly registered `/platform/*` routes. Existing `/auth/*` and all business routes continue to require tenant credentials.

## Bootstrap and audit transaction

Add a restricted operator command under `cmd/` that creates the first account only when none exists. Read the password from a no-echo terminal or a protected input file descriptor; reject password CLI flags and environment variables. Never print the password, hash, connection URL, or token, including on errors. Apply the existing password policy and bcrypt hashing. Use a dedicated provisioning database credential with only the privileges needed for bootstrap, not the schema owner or API runtime credential. A concurrent second bootstrap must fail without replacing the account or emitting a success event.

Insert the account and its `PLATFORM_ADMIN_BOOTSTRAPPED` audit event in one transaction. Platform session creation, refresh, logout, replay containment, and future platform mutations must commit their successful state change and audit event atomically. A failed audit insert fails the operation. Rejected login attempts need bounded, non-identifying security telemetry; audit storage, rate limits, and account-enumeration behavior must be specified together before implementation.

## Migration, rollback, and configuration

Do not edit deployed migrations. The new down migration is for isolated rehearsal; production rollback should keep the additive schema and revert compatible application code, or use a reviewed backup/restore plan if destructive schema rollback is necessary. Update `db/roles/least_privilege.sql` after adding the tables: API runtime may read/write platform sessions and insert audit rows, but cannot update/delete `platform_audit_logs` or create platform accounts. Extend `VerifyLeastPrivilege` and its tests to reject platform audit mutation privileges and elevated role membership. Document provisioning-role grants separately. Add platform signing-secret configuration and a blank example value; never commit a real secret.

## Acceptance criteria

- Exactly one active platform account exists after bootstrap, including under concurrent bootstrap attempts; no HTTP registration or second-account path exists.
- Platform and tenant access/refresh credentials are rejected across auth endpoints and protected routes. Revocation, account status, and replay checks run against platform-only tables.
- Every committed platform state change has its required immutable platform audit row. An audit failure rolls back the change. Runtime credentials cannot update or delete platform audit rows.
- Existing tenant role checks, tenant ownership constraints, and tenant audit foreign keys remain intact.

## Test cases

Fresh and repeated bootstrap; concurrent bootstrap; password input and secret-log scans; wrong-password/unknown-account indistinguishability; expiry, algorithm and audience confusion; platform token on tenant route and tenant token on platform route; refresh/replay/logout races; inactive or revoked session; audit insert failure rollback; audit trigger and runtime-grant rejection; migration up/down/up on an isolated database; existing tenant authentication and isolation regressions.

## Files to change during implementation

`db/migrations/000018_*.{up,down}.sql`, `db/queries/platform_*.sql`, regenerated sqlc output, `internal/security/token.go`, new platform handlers/service/middleware, `internal/server/server.go`, `internal/config/config.go`, `internal/repository/connection.go`, `db/roles/least_privilege.sql`, `api/openapi.yaml` and regenerated OpenAPI code, a restricted `cmd/` bootstrap command, `Makefile`, `.env.example`, database/architecture/operations documentation, and focused unit and PostgreSQL integration tests.

## Risks and unresolved decisions

- A partial unique index alone guarantees at most one account, not at least one. The fixed singleton row plus restricted bootstrap supplies the post-bootstrap invariant. Production API startup rejects an absent account.
- A privileged database owner can disable triggers or alter audit tables. Protect migration/provisioning credentials and retain database backup/log controls.
- Platform and tenant JWTs use separate signing keys. Coordinated key rotation and forced reauthentication remain an operational decision.
- Bootstrap accepts a protected file descriptor and requires the dedicated provisioning role. Define the recovery/password-rotation procedure; the initial-bootstrap command intentionally cannot overwrite the singleton.
- Denied credential logins write an identifier-free `PLATFORM_LOGIN_DENIED` event under the bounded login limiter. Malformed requests and invalid bearer attempts are not persisted as platform audit events; review monitoring needs before expanding this set.

## Branch name

`feature/issue-017-platform-admin-audit`

## Definition of done

The platform identity, sessions, audit, bootstrap, OpenAPI contract, least-privilege grants, and regression tests are reviewed and pass the required release gates. ISSUE-018 may then add platform operations without weakening tenant boundaries.
