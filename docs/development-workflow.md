# Development Workflow

## Branching

Use one issue branch at a time:

- docs/issue-001-requirements
- feature/issue-004-authentication
- feature/issue-009-orders
- feature/issue-014-reports

## Feature workflow

1. Read requirements and acceptance criteria.
2. Create an issue branch from main.
3. Update OpenAPI, migrations, SQL, and tests as needed.
4. Generate sqlc and oapi-codegen output.
5. Implement handlers and service logic.
6. Run gofmt, tests, vet, and lint.
7. Review the diff.
8. Commit and push.
9. Open a pull request.
10. Merge after review and CI pass.

## Required checks

- go build ./...
- go test ./...
- go vet ./...
- golangci-lint run
- migration on an empty database
- integration tests with PostgreSQL
- Swagger endpoint verification

Generated files should be regenerated from their sources and committed when the repository workflow requires them. Never commit secrets.
