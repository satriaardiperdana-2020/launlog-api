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
SELECT orders.id, orders.business_id, orders.outlet_id, orders.customer_id, orders.invoice_number, orders.status,
       orders.payment_status, orders.total_amount, orders.received_at, orders.due_at, orders.completed_at, orders.cancelled_at, orders.created_at, outlet.timezone AS outlet_timezone
FROM orders
JOIN outlets outlet ON outlet.business_id=orders.business_id AND outlet.id=orders.outlet_id
WHERE orders.business_id = sqlc.arg(business_id) AND orders.customer_id = sqlc.arg(customer_id)
  AND (sqlc.arg(is_admin)::boolean OR orders.outlet_id = ANY(sqlc.arg(outlet_ids)::bigint[]))
ORDER BY orders.created_at DESC, orders.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountCustomerOrderHistory :one
SELECT count(*)
FROM orders
WHERE business_id = sqlc.arg(business_id) AND customer_id = sqlc.arg(customer_id)
  AND (sqlc.arg(is_admin)::boolean OR outlet_id = ANY(sqlc.arg(outlet_ids)::bigint[]));
