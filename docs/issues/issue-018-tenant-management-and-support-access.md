# ISSUE-018: Tenant Management and Support Access

## Goal

Let the single `PLATFORM_ADMIN` manage business lifecycle and perform explicitly authorized, traceable support work after the identity and audit foundation in ISSUE-017 exists.

## Scope

In scope: platform-only business listing/detail, tenant provisioning and activation state, first business `ADMIN` provisioning, time-limited support grants, explicit support operations, and platform audit. Out of scope: changing `users.role` to include `PLATFORM_ADMIN`, silently impersonating a business user, unrestricted access to tenant operational data, and any tenant financial mutation through support access.

## API and database design

- Place platform operations under `/platform/businesses`, protected by platform middleware from ISSUE-017. The first contract should include paginated list/detail, create, and activation/deactivation. Platform request bodies must never supply actor IDs, platform roles, audit fields, or arbitrary authorization scopes. OpenAPI must document the separate platform bearer scheme, statuses, and redacted response fields.
- Create a business, initial outlet if required by onboarding, and first tenant `ADMIN` in one transaction with a platform audit event. Preserve `users.business_id`, `users.role` checks, globally unique case-insensitive email, and tenant-aware foreign keys. Deliver any initial password or setup credential through an approved one-time channel; never return or log a stored password. Existing business users continue to authenticate through `/auth/*`.
- Activation changes lock the business row, record previous/new state in allowlisted platform audit metadata, and take effect through the existing active-business checks at tenant login, refresh, and request authentication. Deactivation never deletes tenant records or rewrites historical sessions; it must invalidate effective access immediately. Reactivation does not silently restore expired/revoked credentials.
- Model support authorization explicitly, for example `platform_support_grants` with business ID, purpose/reason, approved scope, creation/expiry/revocation times, and platform actor. A grant is time-limited, targets one business, and is checked again on every support request. An active grant does not create a tenant `users` row or a tenant JWT.
- Expose only named support endpoints whose data scope and redaction are reviewed. Every request must select one target business from the path and verify it against the active grant. Repository queries remain filtered by that business ID; any outlet-level data additionally checks the chosen outlet belongs to it. Default support access is metadata and diagnostic reads, with no customer, order, payment, receipt, or expense payload until each field and purpose is approved.
- Audit grant creation/revocation, business lifecycle changes, support entry, and each support read/action in `platform_audit_logs`. Include actor, business, grant, action, outcome, request ID, and safe metadata. Successful state changes and audit insertion share a transaction. A support read that cannot be audited must fail closed.

## Acceptance criteria

- Tenant credentials cannot call `/platform/*`; platform credentials cannot call tenant routes or be accepted as a tenant `ADMIN`.
- Business provisioning commits its tenant rows and platform audit together, or none of them. Global email uniqueness and last-active-tenant-ADMIN protection remain effective.
- Deactivated businesses fail tenant authentication and protected requests while historical data remains intact.
- Support access requires an unexpired, unrevoked grant for the exact target business and permitted operation. It never gives a platform actor a tenant session.
- Every platform mutation and support access has an immutable, secret-free platform audit record; audit failure blocks the operation.

## Test cases

Duplicate tenant/admin email; concurrent provisioning; partial-provision rollback; cross-business target swap; inactive business; grant expiry and revocation during requests; missing scope; platform/tenant token separation; support read audit failure; sensitive-field redaction; tenant last-admin and outlet-isolation regressions; least-privilege and migration tests.

## Dependencies, risks, and unresolved decisions

- Depends on ISSUE-017 platform identity, session middleware, audit, and runtime grants. Implement new tenant/support schema only in a later additive migration.
- Decide whether support is limited to business metadata and diagnostics, or whether specific tenant data reads are required. Approve each dataset, field redaction, retention period, and purpose before exposing it.
- Decide who can authorize a support grant when exactly one platform admin exists, and whether a second human approval is required outside the API.
- Decide the onboarding credential delivery and reset workflow, whether an initial outlet is mandatory, and whether tenant admins may create additional tenant admins later.
- Specify operational consequences of tenant deactivation, including in-flight writes and background jobs, before implementation.

## Files to change during implementation

An additive migration after ISSUE-017, platform and tenant provisioning/support SQL queries and regenerated sqlc output, platform handlers/service, `internal/server/server.go`, `api/openapi.yaml` and regenerated OpenAPI code, platform audit helpers, grants/least-privilege checks, onboarding documentation, and PostgreSQL integration tests.

## Branch name

`feature/issue-018-tenant-management-support-access`

## Definition of done

The approved platform business and support contract, tenant-safe provisioning, bounded access grants, immutable audit, OpenAPI, and integration regressions pass review and release gates.

## Implemented contract

- Onboarding requires firstOutlet and firstAdmin. Password is write-only and uses
  the approved bcrypt policy; IDs and ADMIN role are server-assigned. Secure
  manual delivery remains an operator responsibility; invitation/reset delivery
  is outside this implementation.
- An authenticated owner creates `/support-requests` with confirmationPassword,
  reason, accessScope READ_ONLY/READ_WRITE, and expiry within one hour. Platform
  start accepts only supportRequestId and cannot expand consent.
- Support endpoints expose diagnostic counts, outlet and perfume metadata.
  READ_WRITE adds only version-checked perfume description edits. Customer data,
  financial mutations, and account/permission changes remain unavailable.
- `/support-audit` and `/audit-logs` provide tenant-filtered history; platform
  audit views are separately authenticated. Support actions carry a distinct
  platform actor and correlate both audit streams without impersonation.
- Revocation is synchronous with shared/exclusive authorization locks: after the
  end/revoke operation commits, later operations fail. An earlier authorized
  operation finishes first, subject to a ten-second transaction timeout and a
  final database-clock expiry check.
