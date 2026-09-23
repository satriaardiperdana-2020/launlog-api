# ISSUE-001: Project Setup and Requirements

## Goal

Provide a reproducible Go 1.27.1, Echo, PostgreSQL 16 backend foundation with safe configuration, health probes, generation tooling, migration tooling, CI, and agreed requirements.

## Scope

In scope: module/toolchain, pgx/v5 pool, environment validation, Asia/Jakarta sessions, `/livez`, `/readyz`, `/health`, graceful shutdown, Make targets, generators, CI, and project documentation. Out of scope: new business behavior, frontend work, and new business schema.

## API/database changes

Adds public operational endpoints only. Adds no business endpoint or business table; it provides tooling to execute existing migrations safely.

## Acceptance criteria

The server validates configuration and pings PostgreSQL before listening; liveness is dependency-free; readiness is bounded and database-backed; shutdown handles SIGINT/SIGTERM; tools are version-pinned; CI builds and verifies migrations on PostgreSQL 16.

## Test cases

Configuration defaults, overrides, dotenv precedence, invalid values, liveness, readiness success/failure, build, vet, generated-code drift, and migration up/down/up verification.

## Branch name

`feature/issue-001-project-setup`

## Definition of done

Documentation, source, Make targets, and CI are committed; `make check` passes; migration verification passes on an isolated database; no secret is tracked.
