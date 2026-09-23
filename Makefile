GO ?= go
GOFMT ?= gofmt
BIN_DIR ?= $(CURDIR)/bin

SQLC_VERSION := v1.31.1
OAPI_CODEGEN_VERSION := v2.7.0
MIGRATE_VERSION := v4.18.1
VULNCHECK_VERSION := v1.8.0
GOLANGCI_LINT_VERSION := v2.13.2

SQLC := $(BIN_DIR)/sqlc
OAPI_CODEGEN := $(BIN_DIR)/oapi-codegen
MIGRATE := $(BIN_DIR)/migrate
VULNCHECK := $(BIN_DIR)/govulncheck
GOLANGCI_LINT := $(BIN_DIR)/golangci-lint
MIGRATION_DATABASE_URL ?= $(DATABASE_URL)

.PHONY: tools fmt fmt-check generate generate-check build test test-race test-integration test-integration-required e2e-smoke migrate-upgrade-check backup-restore-check vet lint vulncheck check release-check run compose-up compose-down migrate-up migrate-down migrate-version migrate-verify
.NOTPARALLEL: release-check

tools: $(SQLC) $(OAPI_CODEGEN) $(MIGRATE)

$(SQLC):
	@mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) $(GO) install github.com/sqlc-dev/sqlc/cmd/sqlc@$(SQLC_VERSION)

$(OAPI_CODEGEN):
	@mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) $(GO) install github.com/oapi-codegen/oapi-codegen/v2/cmd/oapi-codegen@$(OAPI_CODEGEN_VERSION)

$(MIGRATE):
	@mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) $(GO) install -tags 'postgres' github.com/golang-migrate/migrate/v4/cmd/migrate@$(MIGRATE_VERSION)

$(VULNCHECK):
	@mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) $(GO) install golang.org/x/vuln/cmd/govulncheck@$(VULNCHECK_VERSION)

$(GOLANGCI_LINT):
	@mkdir -p $(BIN_DIR)
	GOBIN=$(BIN_DIR) $(GO) install github.com/golangci/golangci-lint/v2/cmd/golangci-lint@$(GOLANGCI_LINT_VERSION)

fmt:
	$(GO) fmt ./...

fmt-check:
	@test -z "$$(find cmd internal tests -name '*.go' -type f -print | xargs -r $(GOFMT) -l)" || (echo 'Go files need gofmt'; exit 1)

generate: tools
	$(OAPI_CODEGEN) --config api/generate.yaml api/openapi.yaml
	$(SQLC) generate

generate-check:
	@temporary_directory=$$(mktemp -d); \
	trap 'rm -rf "$$temporary_directory"' EXIT; \
	cp internal/api/openapi.gen.go "$$temporary_directory/openapi.gen.go"; \
	cp -R internal/repository/postgresql "$$temporary_directory/postgresql"; \
	$(MAKE) generate; \
	diff -q "$$temporary_directory/openapi.gen.go" internal/api/openapi.gen.go; api_status=$$?; \
	diff -qr "$$temporary_directory/postgresql" internal/repository/postgresql; sqlc_status=$$?; \
	test $$api_status -eq 0 && test $$sqlc_status -eq 0

build:
	$(GO) build ./...

test:
	$(GO) test ./...

test-race:
	$(GO) test -race ./...

test-integration:
	@test -n "$(TEST_DATABASE_URL)" || (echo 'TEST_DATABASE_URL is required; refusing to silently skip PostgreSQL integration tests'; exit 1)
	@test -n "$(TEST_ADMIN_DATABASE_URL)" || (echo 'TEST_ADMIN_DATABASE_URL is required for isolated fixture setup/cleanup'; exit 1)
	@test "$(TEST_EXPECT_LEAST_PRIVILEGE)" = "true" || (echo 'TEST_EXPECT_LEAST_PRIVILEGE=true is required'; exit 1)
	$(GO) test -tags=integration ./tests/integration

test-integration-required: test-integration

e2e-smoke:
	@test -n "$(E2E_DATABASE_URL)" || (echo 'E2E_DATABASE_URL is required; use a disposable isolated database'; exit 1)
	@temporary_directory=$$(mktemp -d); \
	trap 'rm -rf "$$temporary_directory"' EXIT; \
	$(GO) build -o "$$temporary_directory/launlog-api" ./cmd/api; \
	APP_BINARY="$$temporary_directory/launlog-api" scripts/api-smoke.sh

migrate-upgrade-check: $(MIGRATE)
	@test -n "$(UPGRADE_MIGRATION_DATABASE_URL)" || (echo 'UPGRADE_MIGRATION_DATABASE_URL is required'; exit 1)
	@test -n "$(UPGRADE_ADMIN_DATABASE_URL)" || (echo 'UPGRADE_ADMIN_DATABASE_URL is required'; exit 1)
	UPGRADE_MIGRATION_DATABASE_URL="$(UPGRADE_MIGRATION_DATABASE_URL)" UPGRADE_ADMIN_DATABASE_URL="$(UPGRADE_ADMIN_DATABASE_URL)" MIGRATE="$(MIGRATE)" scripts/test-migration-upgrade.sh

backup-restore-check:
	@test -n "$(BACKUP_SOURCE_DATABASE_URL)" || (echo 'BACKUP_SOURCE_DATABASE_URL is required'; exit 1)
	@test -n "$(RESTORE_DATABASE_URL)" || (echo 'RESTORE_DATABASE_URL is required and must target a disposable empty database'; exit 1)
	BACKUP_SOURCE_DATABASE_URL="$(BACKUP_SOURCE_DATABASE_URL)" RESTORE_DATABASE_URL="$(RESTORE_DATABASE_URL)" scripts/test-backup-restore.sh

vet:
	$(GO) vet ./...

lint: $(GOLANGCI_LINT)
	$(GOLANGCI_LINT) run --default=none --enable-only=govet,staticcheck,ineffassign,unused ./...

vulncheck: $(VULNCHECK)
	$(VULNCHECK) ./...

check: fmt-check generate-check test vet lint build vulncheck

release-check: check test-integration-required migrate-upgrade-check backup-restore-check e2e-smoke

run:
	$(GO) run ./cmd/api

compose-up:
	docker compose up -d postgres

compose-down:
	docker compose down

migrate-up: $(MIGRATE)
	@test -n "$(MIGRATION_DATABASE_URL)" || (echo 'MIGRATION_DATABASE_URL (or DATABASE_URL for local development) is required'; exit 1)
	$(MIGRATE) -path db/migrations -database "$(MIGRATION_DATABASE_URL)" up

migrate-down: $(MIGRATE)
	@test -n "$(MIGRATION_DATABASE_URL)" || (echo 'MIGRATION_DATABASE_URL (or DATABASE_URL for local development) is required'; exit 1)
	@test "$(CONFIRM_MIGRATE_DOWN)" = "1" || (echo 'Set CONFIRM_MIGRATE_DOWN=1 to run destructive down migrations'; exit 1)
	$(MIGRATE) -path db/migrations -database "$(MIGRATION_DATABASE_URL)" down -all

migrate-version: $(MIGRATE)
	@test -n "$(MIGRATION_DATABASE_URL)" || (echo 'MIGRATION_DATABASE_URL (or DATABASE_URL for local development) is required'; exit 1)
	$(MIGRATE) -path db/migrations -database "$(MIGRATION_DATABASE_URL)" version

migrate-verify:
	$(MAKE) migrate-up MIGRATION_DATABASE_URL="$(or $(MIGRATION_DATABASE_URL),$(DATABASE_URL))"
	$(MAKE) migrate-down MIGRATION_DATABASE_URL="$(or $(MIGRATION_DATABASE_URL),$(DATABASE_URL))" CONFIRM_MIGRATE_DOWN=1
	$(MAKE) migrate-up MIGRATION_DATABASE_URL="$(or $(MIGRATION_DATABASE_URL),$(DATABASE_URL))"
