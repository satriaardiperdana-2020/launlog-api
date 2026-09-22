# Launlog API

Backend foundation for the Launlog laundry POS. The API is implemented with Go, Echo, PostgreSQL, sqlc, and OpenAPI.

## Requirements

- Go 1.21 or newer
- PostgreSQL 16, or Docker Compose

## Local setup

1. Copy `.env.example` to `.env.development` and adjust values for your environment.
2. Start PostgreSQL with `docker compose up -d postgres`.
3. Start the API with `make run`.

The API automatically loads `.env.<APP_ENV>` (default `.env.development`) and falls back to `.env`. Explicitly exported environment variables take precedence over dotenv files.

The API listens on `HTTP_HOST:HTTP_PORT` (default `0.0.0.0:8080`).

## Health check

```shell
curl http://localhost:8080/health
```

The endpoint returns HTTP 200 with `{"status":"ok"}` when PostgreSQL is reachable, or HTTP 503 with `{"status":"unavailable"}` when it is not.

## Development checks

```shell
make check
```

This formats the Go source and runs unit tests, `go vet`, and `go build`. Project requirements and implementation planning are maintained in `docs/`.
