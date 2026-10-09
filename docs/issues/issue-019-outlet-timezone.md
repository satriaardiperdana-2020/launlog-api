# ISSUE-019: Outlet timezone

## Goal

Persist an IANA timezone per outlet and expose it to clients without changing order instants.

## Contract

Migration 000020 adds outlets.timezone and backfills existing outlets to Asia/Jakarta. Database default and non-null/nonempty constraints protect stored metadata. API onboarding, POST /outlets, and PUT /outlets/{outletId} normalize omitted, null, empty, or whitespace to Asia/Jakarta; explicit names are trimmed and validated with time.LoadLocation. Local and unknown zones return INVALID_TIMEZONE (400). Embedded tzdata makes validation portable. PUT omission deliberately resets timezone to the default.

Outlet responses (including platform onboarding/support) include timezone. Tenant /auth/me and login/refresh user responses include outlets [{id, timezone}] for active assigned outlets, preserving outletIds. Metadata confers no authority. Orders, order summaries, and customer order history include the current outlet_timezone. Timezone is read from the database, not JWT claims or an order snapshot.

Timestamp fields remain RFC3339 with an explicit offset or Z and the same stored TIMESTAMPTZ instant. Updating an outlet does not update orders. This issue does not change invoice date allocation, dashboard/report day boundaries, expense date filters, or receipt message formatting; those retain their existing Jakarta policy and require a separate contract.

## Acceptance criteria and tests

- Existing outlets default to Jakarta; omitted/null/empty/whitespace default on create, update, and platform onboarding.
- Makassar and Jayapura remain selected; invalid names and JSON types fail without writes.
- Outlet and current-user metadata remain tenant/assignment scoped; token separation and support authorization are unchanged.
- Outlet timezone writes and audits commit or roll back together.
- Order create/detail/list/history/idempotency replay return current timezone with unchanged RFC3339 instants, including near midnight.
- Migration up/down/up, legacy upgrade backfill, least-privilege integration, backup/restore, and smoke checks pass on isolated PostgreSQL.

## Branch

feature/outlet-timezone
