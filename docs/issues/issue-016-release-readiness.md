# ISSUE-016: Integration Tests and Release Readiness

## Goal

Prove the backend is deployable, reproducible, and safe for frontend integration.

## Scope

In scope: PostgreSQL integration suites, migration verification, generated-code checks, CI gates, release documentation, and operational runbooks. Out of scope: frontend implementation.

## API/database changes

No product feature change is required; this issue may add test fixtures, CI workflows, and documentation.

## Acceptance criteria

CI runs build, tests, vet, generator drift, migration up/down/up, and integration suites; documented configuration contains no secrets; release smoke checks cover liveness/readiness and OpenAPI.

## Test cases

Fresh clone/tool installation, empty-database migration, API smoke test, authentication/business-isolation regression suite, and rollback verification.

## Branch name

`feature/issue-016-release-readiness`

## Definition of done

All required checks are green, release notes/runbook are approved, and the backend release gate is signed off.
