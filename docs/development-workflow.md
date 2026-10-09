# Development Workflow

## Branching

Use one branch per issue, based on current `main`: `feature/issue-001-project-setup`, `feature/issue-002-migrations-and-ownership`, and so on. The exact branch for every issue is recorded in `docs/issues/`.

## Feature workflow

1. Read requirements and acceptance criteria.
2. Create an issue branch from main.
3. Update OpenAPI, migrations, SQL, and tests as needed.
4. Run `make generate` when OpenAPI, migrations, or sqlc queries change.
5. Implement handlers and service logic.
6. Run `make check`; run `make migrate-verify` against an isolated database when migrations change.
7. Review the diff.
8. Commit and push.
9. Open a pull request.
10. Merge after review and CI pass.

## Required checks

- `make check` for formatting, generated-code drift, tests, `go vet`, pinned golangci-lint, build, and govulncheck.
- `make release-check` before a release candidate; it requires isolated PostgreSQL databases for integration, upgrade, restore, and process-level HTTP smoke tests.
- `make migrate-verify` for migration changes, using a disposable database only.
- Inspect generated OpenAPI output and exercise changed endpoints with curl or an HTTP test.
- CI must be green before merge.

Generator versions are pinned in the Makefile. Generated files are committed after regeneration. Never commit secrets, local dotenv files, the `bin/` tool directory, or production database URLs.

In deployed environments use a separate `MIGRATION_DATABASE_URL` and `DATABASE_URL`; the first is a schema owner and the second is the verified least-privilege runtime role. Local Compose credentials are for development only. `make check` includes the pinned `govulncheck` scan against the Go vulnerability database.

PostgreSQL tests are never considered passed when skipped. `make test-integration` requires `TEST_DATABASE_URL`, `TEST_ADMIN_DATABASE_URL`, and `TEST_EXPECT_LEAST_PRIVILEGE=true`. The release gate additionally requires the upgrade, restore, and E2E database variables documented in [release readiness](release-readiness.md).

## ISSUE-018 deployment

1. Back up the database and rehearse migration 19 up/down/up on an isolated database.
2. Apply migration 19 as `launlog_owner`, then reapply `db/roles/least_privilege.sql`
   as DBA. Runtime remains `launlog_runtime`; bootstrap must have no owner/runtime
   membership or administrative privileges.
3. Deploy the API and run platform/tenant credential-separation smoke checks.
4. Provision initial owners only through authenticated platform onboarding. Supply
   the initial password through an approved secure owner/operator channel over
   TLS; it is write-only and must not appear in shell history, tickets, logs, or
   audit. Automated invitation delivery and password reset are not implemented.
5. Production rollback reverts application code while retaining the additive
   schema and immutable audit history; destructive down migration is rehearsal only.

Onboarding has no automatic retry/idempotency key. After a network timeout, inspect
business metadata and email availability before retrying. Duplicate email yields
409 without disclosing its owning tenant. A retry must never overwrite credentials.

## Outlet timezone deployment (ISSUE-019)

Apply additive migration 20 with the migration owner before deploying the API;
rehearse up/down/up and legacy backfill on isolated databases. Existing outlets
are backfilled to Asia/Jakarta. Roll back the API before dropping the column;
the down migration discards selected outlet timezones. Clients using full PUT
must send their selected timezone or it resets to Jakarta. Generated sqlc and
OpenAPI artifacts are regenerated with make generate; never edit them by hand.
Order instants, invoice/report day policy, and tenant permissions are preserved.
