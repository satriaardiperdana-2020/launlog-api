GO ?= go
GOFMT ?= gofmt
BIN_DIR ?= $(CURDIR)/bin

SQLC_VERSION := v1.31.1
OAPI_CODEGEN_VERSION := v2.7.0
MIGRATE_VERSION := v4.18.1

SQLC := $(BIN_DIR)/sqlc
OAPI_CODEGEN := $(BIN_DIR)/oapi-codegen
MIGRATE := $(BIN_DIR)/migrate

.PHONY: tools fmt fmt-check generate generate-check build test test-race test-integration vet check run compose-up compose-down migrate-up migrate-down migrate-version migrate-verify

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
	$(GO) test -tags=integration ./tests/integration

vet:
	$(GO) vet ./...

check: fmt-check generate-check test vet build

run:
	$(GO) run ./cmd/api

compose-up:
	docker compose up -d postgres

compose-down:
	docker compose down

migrate-up: $(MIGRATE)
	@test -n "$(DATABASE_URL)" || (echo 'DATABASE_URL is required'; exit 1)
	$(MIGRATE) -path db/migrations -database "$(DATABASE_URL)" up

migrate-down: $(MIGRATE)
	@test -n "$(DATABASE_URL)" || (echo 'DATABASE_URL is required'; exit 1)
	@test "$(CONFIRM_MIGRATE_DOWN)" = "1" || (echo 'Set CONFIRM_MIGRATE_DOWN=1 to run destructive down migrations'; exit 1)
	$(MIGRATE) -path db/migrations -database "$(DATABASE_URL)" down -all

migrate-version: $(MIGRATE)
	@test -n "$(DATABASE_URL)" || (echo 'DATABASE_URL is required'; exit 1)
	$(MIGRATE) -path db/migrations -database "$(DATABASE_URL)" version

migrate-verify:
	$(MAKE) migrate-up DATABASE_URL="$(DATABASE_URL)"
	$(MAKE) migrate-down DATABASE_URL="$(DATABASE_URL)" CONFIRM_MIGRATE_DOWN=1
	$(MAKE) migrate-up DATABASE_URL="$(DATABASE_URL)"
