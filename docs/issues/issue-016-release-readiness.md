# ISSUE-016: Integration Tests and Release Readiness

## Goal

Prove the backend is deployable, reproducible, and safe for frontend integration.

## Scope

In scope: PostgreSQL integration suites, migration verification, generated-code checks, CI gates, release documentation, and operational runbooks. Out of scope: frontend implementation.

## API/database changes

No product feature change is required; this issue adds shared production-router test wiring, isolated PostgreSQL rehearsal scripts, stricter CI gates, and operational documentation. No business schema or frontend code is added.

## Acceptance criteria

CI runs build, tests, vet, lint, dependency scanning, generator drift, migration up/down/up, real-PostgreSQL feature/concurrency tests through the production router, a data-preserving upgrade from migration 6, backup/restore, and a process-level HTTP smoke test. PostgreSQL gates fail if database credentials are missing; skipped integration tests are not treated as release evidence. Configuration examples contain no secrets; OpenAPI is parsed and regenerated reproducibly.

## Test cases

Fresh clone/tool installation, empty-database migration, upgrade with legacy data/session backfill, actual binary HTTP smoke test, authentication/business-isolation and concurrency regressions, backup/restore into a separate database, generated OpenAPI/sqlc reproducibility, and documented application/schema rollback rehearsal.

## Branch name

`feature/issue-016-release-readiness`

## Definition of done

All CI release gates and documented test/staging rehearsals are green, release notes/runbook are approved, and the backend release gate is signed off. Production readiness is not claimed until a deployment operator executes and approves the deployment-specific checklist in `docs/release-readiness.md`.
