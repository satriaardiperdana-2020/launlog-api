# ISSUE-016: Release and Operations Readiness

## Acceptance evidence across ISSUE-001–016

The feature integration tests use PostgreSQL and now instantiate the exact production Echo router from `internal/server.New`; they exercise handlers, auth middleware, permission middleware, tenant/outlet checks, and persistence over HTTP requests. `scripts/api-smoke.sh` additionally launches the built production executable and sends requests over TCP. These tests do not prove cloud networking, deployed secrets, or operational ownership.

| Issue | Verification evidence | Gate |
| --- | --- | --- |
| 001 Setup | `internal/config` tests, health handler tests, executable smoke (liveness/readiness/SIGTERM), build/vet/lint | `make check`, `make e2e-smoke` |
| 002 Migrations/ownership | Migration up/down/up, cross-business FK/tenant fixture, ownership metadata assertions | CI migrations job, `make migrate-upgrade-check` |
| 003 Authentication | Algorithm/expiry/password unit tests; HTTP auth, replay, rotation/logout race, disabled account/business/outlet, rate/body limit, and cross-tenant tests | `make test-integration-required` |
| 004 Outlets/staff/permissions | HTTP management, permission/tenant checks, last-owner race, and audit assertions | `make test-integration-required` |
| 005 Customers | HTTP lifecycle, paging/history, permission/tenant scope, non-unique phone and audit redaction | `make test-integration-required` |
| 006 Services | HTTP catalog CRUD/filter/validation, tenant/permission checks, immutable order snapshots | `make test-integration-required` |
| 007 Perfumes | HTTP CRUD/search/deactivation/soft deletion, duplicate-name and permission/tenant isolation, audit coverage | `make test-integration-required` |
| 008 Orders/invoices | HTTP order flow, exact totals/snapshots/idempotency, invoice allocation and reconciliation fixtures | `make test-integration-required` |
| 009 Status/cancellation | HTTP state transitions, cancellation preservation, concurrent order locking and history | `make test-integration-required` |
| 010 Payments | HTTP partial/settled payments, exact arithmetic, idempotency, overpayment/cash-change behavior and payment/cancellation concurrency | `make test-integration-required` |
| 011 Expenses | HTTP categories and expense lifecycle, Jakarta date default, ETag lost-update race, atomic redacted audit | `make test-integration-required` |
| 012 Dashboard | HTTP dashboard with separately aggregated confirmed payments and expenses using local-day bounds | `make test-integration-required` |
| 013 Reports | HTTP six-report reconciliation fixture, date basis, customer distinct counts, deterministic rankings | `make test-integration-required` |
| 014 Receipts/settings | HTTP receipt snapshot and QR auth, template placeholder allowlist, tenant/permission checks | `make test-integration-required` |
| 015 Audit/security | session/customer audit redaction, database runtime privilege assertions, headers/CORS tests, govulncheck | `make check`, `make test-integration-required` |
| 016 Release | Upgrade from migration 6 preserving legacy business/session data, pg_dump/pg_restore rehearsal, actual process smoke | `make release-check` |

The table maps each issue to executable evidence; it is not a substitute for a human review of changed requirements. CI integration gates require the test database, separate test-admin database credential, and least-privilege assertion flag. They fail before running when these are absent; a `go test -tags=integration` run that reports skips is not release evidence.

## Release gate

Use Go 1.27.1 and PostgreSQL 16. From a clean checkout, install the pinned tools and run:

```sh
make tools
make release-check
```

`make release-check` runs formatting and OpenAPI/sqlc generator reproducibility, unit tests, `go vet`, pinned golangci-lint, build, govulncheck, required PostgreSQL feature/concurrency tests, migration upgrade, backup/restore, and a real process HTTP smoke test. Its database environment variables must reference disposable isolated databases:

- `TEST_DATABASE_URL`: API/runtime test role on the fully migrated feature-test database.
- `TEST_ADMIN_DATABASE_URL`: test-only administrator credential for fixture cleanup and schema assertions; never used by API requests.
- `TEST_EXPECT_LEAST_PRIVILEGE=true`: requires runtime-role privilege verification rather than skipping it.
- `E2E_DATABASE_URL`: runtime-role connection used by the actual server process smoke test.
- `UPGRADE_MIGRATION_DATABASE_URL`: migrator credential on a new, empty upgrade rehearsal database.
- `UPGRADE_ADMIN_DATABASE_URL`: test administrator on that same upgrade database.
- `BACKUP_SOURCE_DATABASE_URL`: test administrator on a disposable, fully migrated source database.
- `RESTORE_DATABASE_URL`: test administrator on a separate disposable restore target.

The scripts reject an already initialized migration-upgrade database and do not target a configured production endpoint. Keep credentials in ephemeral CI variables or a secret manager; never place them in `.env.example`, command-line history, or Git. CI uses three distinct database identities (runtime, migrator, administrator) with throwaway credentials and PostgreSQL service databases. `TEST_ADMIN_DATABASE_URL`, upgrade admin, and restore credentials exist only to make isolated test setup possible.

## Deployment procedure

1. Build from the reviewed commit and publish an immutable image/binary digest. Keep the previous artifact available.
2. Provision PostgreSQL 16, TLS with hostname verification, and the separate roles described in [database operations](database.md#runtime-database-role). Inject `DATABASE_URL`, `MIGRATION_DATABASE_URL`, and JWT signing secret using the deployment secret manager. Do not reuse local Compose credentials.
3. Take a provider-managed, encrypted, point-in-time backup before schema changes and confirm its retention and restore access. Do not use `make migrate-verify` against staging or production.
4. Run `make migrate-up` once as the migration role from the release job. Verify the migration version is 16 and `dirty=false`; review migration output for errors.
5. Deploy the immutable application artifact with the runtime role. Production startup must pass verified TLS and least-privilege checks before listening.
6. Verify `GET /livez`, `GET /readyz`, and `GET /health` through the intended load balancer. Check application startup/shutdown logs, database pool saturation/connection errors, authentication failure rates, and that secrets/tokens/PII are not logged.
7. Run an authenticated smoke test from a restricted operator client: login, `/auth/me`, a tenant-scoped read, then logout. Confirm a revoked access token fails. Avoid creating business data just for a probe.
8. Keep the release under observation for the agreed operational window. Record image digest, migration version, operator, check results, backup identifier, and go/no-go decision in the deployment record.

## Operational checks and current limits

- Liveness (`/livez`) indicates only that the process is alive. Readiness (`/readyz`) and compatibility health (`/health`) make a bounded database ping; a ready process does not certify every downstream operation.
- Logs are structured JSON. The application does not currently expose a Prometheus metrics endpoint; use platform process/network telemetry and PostgreSQL monitoring for CPU, memory, connection use, lock waits, disk, replication lag, and backup health.
- Auth abuse-limit storage is bounded but process-local. Horizontal replicas have independent buckets; enforce upstream edge/WAF limits as well, and do not treat this in-process limit as a distributed quota.
- Automated bank verification, refund processing, message delivery, printer discovery, and client device actions are not implemented. Refund policy remains unapproved.
- Production deployment, cloud IAM/secrets, TLS termination, backup retention, restore RTO/RPO, alert routing, and operator approval are environment-specific and are not proven by repository CI.

## Rollback strategy

Prefer application rollback to the previous immutable artifact only when that artifact is compatible with the current additive schema. Do not run down migrations in production as an automatic deployment rollback; several downs are intentionally destructive or can fail after valid newer data is written. Keep changes expand/contract: add compatible schema first, deploy compatible code, then remove obsolete schema in a later explicitly approved migration.

If a release causes data corruption or requires schema rollback, stop writes, preserve logs and the current database snapshot, page the database operator, and restore the encrypted pre-deploy backup/PITR into a separate database first. Validate migration version, critical table counts, tenant relationships, and application smoke checks against the restored target before a deliberate cutover. Record the recovery point and data-loss window; never overwrite the only source backup during rehearsal. Reapply runtime/migrator grants and verify the least-privilege startup check after restore.

CI's `scripts/test-backup-restore.sh` is a rehearsal of custom-format `pg_dump` plus `pg_restore` into a separate empty disposable database. It validates schema version and required tables; it is not a substitute for scheduled provider-level PITR restoration, encryption/key recovery, or an approved production RTO/RPO exercise.

## Release decision

CI can establish code, PostgreSQL concurrency, migration, generator, and disposable restore correctness. Production readiness remains **not approved** until the target-environment deployment, provider backup/restore rehearsal, alerting, access review, and operator sign-off above have been completed and recorded. No frontend work is part of this release issue.
