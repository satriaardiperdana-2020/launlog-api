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
