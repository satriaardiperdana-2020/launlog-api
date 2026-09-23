# Launlog API

The source of truth is [api/openapi.yaml](../api/openapi.yaml). The current server base path is `/`; versioning is deferred until a versioned public API is introduced.

## Foundation endpoints

- `GET /livez` is public process liveness and always returns `200 {"status":"ok"}` while Echo is running.
- `GET /readyz` is public database readiness. It returns `200 {"status":"ok"}` when PostgreSQL responds within `READINESS_TIMEOUT`, otherwise `503 {"status":"unavailable"}`.
- `GET /health` is a compatibility alias for `/readyz`.

## Existing access endpoints

- `POST /auth/login` and `POST /auth/refresh` are public.
- `POST /auth/logout` and `GET /auth/me` require `Authorization: Bearer <access-token>`.

## Contract rules

- Add all endpoint changes to OpenAPI before generating Echo types with `make generate`.
- IDs and rupiah values are signed 64-bit JSON integers. Decimal quantities are strings with up to three fractional digits.
- Date-times are RFC 3339 values with an explicit offset; business-facing values use Asia/Jakarta.
- Do not accept actor, user, business, or outlet authority from request JSON. Derive it from authenticated context.
- List endpoints added by later issues must use the shared `page` and `pageSize` conventions.
