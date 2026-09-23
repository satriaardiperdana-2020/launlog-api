-- name: ListCustomers :many
SELECT id, business_id, name, phone, address, created_at, updated_at
FROM customers
WHERE business_id = sqlc.arg(business_id) AND deleted_at IS NULL
  AND (sqlc.arg(search)::text = '' OR name ILIKE '%' || sqlc.arg(search)::text || '%' OR COALESCE(phone, '') ILIKE '%' || sqlc.arg(search)::text || '%')
ORDER BY lower(name), id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountCustomers :one
SELECT count(*)
FROM customers
WHERE business_id = sqlc.arg(business_id) AND deleted_at IS NULL
  AND (sqlc.arg(search)::text = '' OR name ILIKE '%' || sqlc.arg(search)::text || '%' OR COALESCE(phone, '') ILIKE '%' || sqlc.arg(search)::text || '%');

-- name: GetCustomer :one
SELECT id, business_id, name, phone, address, created_at, updated_at
FROM customers
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(customer_id) AND deleted_at IS NULL;

-- name: GetCustomerForUpdate :one
SELECT id, business_id, name, phone, address, created_at, updated_at
FROM customers
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(customer_id) AND deleted_at IS NULL
FOR UPDATE;

-- name: GetCustomerForHistory :one
SELECT id
FROM customers
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(customer_id);

-- name: CreateCustomer :one
INSERT INTO customers (business_id, name, phone, address)
VALUES (sqlc.arg(business_id), sqlc.arg(name), sqlc.arg(phone), sqlc.arg(address))
RETURNING id, business_id, name, phone, address, created_at, updated_at;

-- name: UpdateCustomer :one
UPDATE customers
SET name = sqlc.arg(name), phone = sqlc.arg(phone), address = sqlc.arg(address), updated_at = now()
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(customer_id) AND deleted_at IS NULL
RETURNING id, business_id, name, phone, address, created_at, updated_at;

-- name: DeactivateCustomer :one
UPDATE customers
SET deleted_at = now(), updated_at = now()
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(customer_id) AND deleted_at IS NULL
RETURNING id, business_id, name, phone, address, created_at, updated_at, deleted_at;

-- name: ListCustomerOrderHistory :many
SELECT id, business_id, outlet_id, customer_id, invoice_number, status,
       payment_status, total_amount, received_at, due_at, completed_at, cancelled_at, created_at
FROM orders
WHERE business_id = sqlc.arg(business_id) AND customer_id = sqlc.arg(customer_id)
  AND (sqlc.arg(is_admin)::boolean OR outlet_id = ANY(sqlc.arg(outlet_ids)::bigint[]))
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountCustomerOrderHistory :one
SELECT count(*)
FROM orders
WHERE business_id = sqlc.arg(business_id) AND customer_id = sqlc.arg(customer_id)
  AND (sqlc.arg(is_admin)::boolean OR outlet_id = ANY(sqlc.arg(outlet_ids)::bigint[]));
