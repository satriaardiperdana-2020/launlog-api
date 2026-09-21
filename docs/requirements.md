# Launlog Requirements

## Product scope

Launlog is a laundry POS for a laundry outlet. The backend is implemented first with Go, Echo, PostgreSQL, sqlc, OpenAPI, and Swagger. The Vue frontend is developed later in launlog-app.

## Roles

- ADMIN: owner with full access within the business, including all Laundry Staff operations.
- LAUNDRY_STAFF: daily laundry and POS operations according to assigned permissions.

The application does not require a separate CASHIER role for the MVP.

## MVP features

- Login and refresh sessions
- Business and outlet profile
- Customers: create, search, detail, update, and order history
- Services: kilogram, piece, meter, and square meter pricing
- Perfumes
- Orders and order status history
- Payments: cash, BCA transfer, and QRIS recording
- Unpaid, partially paid, paid, and refunded payment states
- Expenses and expense categories
- Cancelled orders with soft deletion and cancellation reason
- Dashboard income and expense totals
- Income, expense, profit/loss, order, cancellation, and customer reports
- Receipt data, QR identifier, WhatsApp template, and printer settings metadata
- Staff permissions and audit history

## Deferred

Customer deposits, quota packages, subscription billing, automatic BCA inquiry, automatic WhatsApp provider integration, and advanced visual analytics.

## Order workflow

RECEIVED -> PROCESSING -> READY_FOR_PICKUP -> COMPLETED

An order may also be CANCELLED. Every transition records the actor and timestamp.

## Business rules

1. The backend is the authority for authentication, authorization, totals, payment status, and tenant isolation.
2. Every business-owned record is scoped by business_id; outlet records also use outlet_id.
3. Income is based on confirmed payments by payment date, not merely order totals.
4. Historical order items preserve the service name, unit, quantity, and price used at order time.
5. Cancelled records are retained for history and reporting.
6. Money uses exact rupiah amounts; weight and quantity support decimals.
7. Deposits and quota packages are not implemented in the MVP.

## Acceptance criteria

The backend must pass authentication, business isolation, order/payment atomicity, cancellation, report reconciliation, migration, and Swagger tests before frontend feature development begins.
