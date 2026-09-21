# Launlog Roadmap

## Timeline

The schedule is expressed as relative weeks from project kickoff. Phase B starts only after the Phase A release gate passes.

| Period | Milestone | Scope |
| --- | --- | --- |
| Weeks 1–2 | Foundation | Requirements, repository setup, PostgreSQL migrations, ownership, sqlc, and OpenAPI generation |
| Weeks 3–4 | Core access and catalog | Authentication, permissions, customers, services, and perfumes |
| Weeks 5–7 | Order lifecycle | Orders, invoice numbers, status history, cancellations, and payments |
| Weeks 8–9 | Operations and reporting | Expenses, dashboard, and reports |
| Week 10 | Delivery features | Receipts, QR, WhatsApp templates, and printer settings |
| Weeks 11–12 | Backend release gate | Integration tests, CI, security, and release preparation |
| Weeks 13–14 | Frontend foundation | Vue foundation, API client, authentication, layout, and route guards |
| Weeks 15–17 | Frontend operations | Dashboard, customers, services, perfumes, orders, payments, cancellations, and expenses |
| Weeks 18–19 | Frontend completion | Reports, settings, QR scanning, printer integration, and WhatsApp sharing |

## Phase A: Backend

1. Requirements and repository setup
2. PostgreSQL migrations and ownership
3. sqlc and OpenAPI generation
4. Authentication and permissions
5. Customers
6. Services and perfumes
7. Orders and invoice numbers
8. Status history and cancellations
9. Payments
10. Expenses and dashboard
11. Reports
12. Receipts, QR, WhatsApp templates, and printer settings
13. Integration tests, CI, security, and release

## Phase B: Frontend

After the backend release gate passes:

1. Vue foundation
2. API client and authentication
3. Layout and route guards
4. Dashboard
5. Customers
6. Services and perfumes
7. Orders
8. Payments and cancellation
9. Expenses
10. Reports
11. Settings
12. QR scanning, printer integration, and WhatsApp sharing

## Release rule

Frontend feature work does not begin until backend APIs, permissions, migrations, and report calculations have been tested.
