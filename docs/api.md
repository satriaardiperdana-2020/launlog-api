# Launlog API

The source of truth is [api/openapi.yaml](../api/openapi.yaml). The current server base path is `/`; versioning is deferred until a versioned public API is introduced.

## Foundation endpoints

- `GET /livez` is public process liveness and always returns `200 {"status":"ok"}` while Echo is running.
- `GET /readyz` is public database readiness. It returns `200 {"status":"ok"}` when PostgreSQL responds within `READINESS_TIMEOUT`, otherwise `503 {"status":"unavailable"}`.
- `GET /health` is a compatibility alias for `/readyz`.

## Existing access endpoints

- `POST /auth/login` and `POST /auth/refresh` are public.
- `POST /auth/logout` and `GET /auth/me` require `Authorization: Bearer <access-token>`.
- Auth request bodies are capped at 4096 bytes. One per-process limiter allows 10 requests per minute for each direct TCP peer and route; forwarding headers are not trusted. A full 4096-key limiter rejects new keys until entries expire.
- Credential failures use the same 401 response. A valid, cost-12 dummy bcrypt hash is checked when the email has no active account.
- At password creation or change, require 8–128 Unicode characters and at most 72 UTF-8 bytes. Login checks existing passwords without imposing that character minimum; bcrypt's byte limit still applies. OpenAPI `minLength`/`maxLength` count characters, not UTF-8 bytes.
- Rotating refresh tokens retain consumed hashes to detect replay. Replay revokes the session family. If a committed rotation response is lost, the client must log in again because the raw replacement token is not stored.

## Contract rules

- Add all endpoint changes to OpenAPI before generating Echo types with `make generate`.
- IDs and rupiah values are signed 64-bit JSON integers. Decimal quantities are strings with up to three fractional digits.
- Date-times are RFC 3339 values with an explicit offset; business-facing values use Asia/Jakarta.
- Do not accept actor, user, business, or outlet authority from request JSON. Derive it from authenticated context.
- List endpoints added by later issues must use the shared `page` and `pageSize` conventions.
