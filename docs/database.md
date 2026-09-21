# Launlog Database Design

## Table groups

### Ownership and access

- businesses
- outlets
- users
- user_outlets
- permissions
- user_permissions
- refresh_tokens

### Master data

- customers
- services
- perfumes
- payment_methods

### Orders

- orders
- order_items
- order_status_history
- invoice_counters

### Finance

- payments
- refunds
- expense_categories
- expenses

### Settings and history

- receipt_templates
- printer_settings
- audit_logs

## Rules

- Use UUID or BIGINT consistently across all tables.
- Every business-owned table contains business_id.
- Outlet-specific tables contain outlet_id.
- Store money as BIGINT rupiah amounts.
- Store quantity/weight as NUMERIC(12,3).
- Store estimated duration as minutes.
- Keep service snapshots in order_items.
- Use migrations; do not repeatedly edit an applied migration.
- Use status plus soft-delete metadata for cancelled records.
- Add foreign-key constraints and indexes for business_id, outlet_id, dates, status, and invoice number.

## Migration order

1. Ownership and users
2. Permissions and sessions
3. Customers, services, perfumes, payment methods
4. Orders and status history
5. Payments and refunds
6. Expenses
7. Settings and audit
