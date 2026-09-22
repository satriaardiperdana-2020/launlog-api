.PHONY: fmt build test test-integration vet run check

fmt:
	gofmt -w $$(find cmd internal tests -name '*.go' -type f)

build:
	go build ./...

test:
	go test ./...

test-integration:
	go test -tags=integration ./...

vet:
	go vet ./...

run:
	go run ./cmd/api

check: fmt test vet build
