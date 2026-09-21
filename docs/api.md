# Launlog API

Base path: /api/v1

## Public endpoints

- GET /health
- POST /auth/login
- POST /auth/refresh
- POST /auth/logout
- POST /auth/me or GET /auth/me, according to the finalized OpenAPI contract

## Protected modules

- businesses and outlets
- staff and permissions
- customers
- services and perfumes
- orders
- order status history
- payments
- expenses
- dashboard
- reports
- receipt and QR lookup
- settings

## API rules

- Define all requests and responses in api/openapi.yaml.
- Do not accept actor user_id from request JSON.
- Derive the actor from authenticated context.
- Return consistent error responses.
- Add pagination to list endpoints.
- Use ISO date/time values and explicit timezone behavior.
- Generate Echo interfaces with oapi-codegen.
- Verify every endpoint in Swagger and curl.
