-- name: LockOrderIdempotency :exec
SELECT pg_advisory_xact_lock(hashtextextended(sqlc.arg(lock_key)::text, 0));

-- name: GetOrderByIdempotencyKey :one
SELECT id, business_id, outlet_id, customer_id, invoice_number, status, payment_status,
       total_amount, notes, received_at, due_at, created_by, created_at, updated_at, request_hash
FROM orders
WHERE business_id = sqlc.arg(business_id) AND outlet_id = sqlc.arg(outlet_id)
  AND idempotency_key = sqlc.arg(idempotency_key);

-- name: GetActiveOrderOutlet :one
SELECT id, code, name
FROM outlets
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(outlet_id) AND is_active
FOR SHARE;

-- name: GetActiveStaffOrderOutlet :one
SELECT u.user_id
FROM user_outlets AS u
JOIN outlets AS o ON o.business_id = u.business_id AND o.id = u.outlet_id
WHERE u.business_id = sqlc.arg(business_id) AND u.user_id = sqlc.arg(user_id)
  AND u.outlet_id = sqlc.arg(outlet_id) AND o.is_active
FOR SHARE OF u, o;

-- name: GetActiveOrderCustomer :one
SELECT id, name
FROM customers
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(customer_id)
  AND deleted_at IS NULL
FOR SHARE;

-- name: GetActiveOrderService :one
SELECT id, business_id, name, unit, unit_price_amount
FROM services
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(service_id)
  AND is_active AND deleted_at IS NULL
FOR SHARE;

-- name: GetActiveOrderPerfume :one
SELECT id, business_id, name
FROM perfumes
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(perfume_id)
  AND is_active AND deleted_at IS NULL
FOR SHARE;

-- name: GetOrderTime :one
SELECT now()::timestamptz AS received_at, CURRENT_DATE::date AS counter_date;

-- name: AllocateInvoiceNumber :one
INSERT INTO invoice_counters (business_id, outlet_id, counter_date, next_number)
VALUES (sqlc.arg(business_id), sqlc.arg(outlet_id), sqlc.arg(counter_date), 2::bigint)
ON CONFLICT (business_id, outlet_id, counter_date)
DO UPDATE SET next_number = invoice_counters.next_number + 1, updated_at = now()
RETURNING counter_date, (next_number - 1)::bigint AS invoice_sequence;

-- name: CreateOrder :one
INSERT INTO orders (
    business_id, outlet_id, customer_id, invoice_number, total_amount, notes,
    received_at, due_at, created_by, idempotency_key, request_hash
)
VALUES (
    sqlc.arg(business_id), sqlc.arg(outlet_id), sqlc.arg(customer_id), sqlc.arg(invoice_number),
    sqlc.arg(total_amount), sqlc.arg(notes), sqlc.arg(received_at), sqlc.arg(due_at),
    sqlc.arg(created_by), sqlc.arg(idempotency_key), sqlc.arg(request_hash)
)
RETURNING id, business_id, outlet_id, customer_id, invoice_number, status, payment_status,
          total_amount, notes, received_at, due_at, created_by, created_at, updated_at;

-- name: CreateOrderItem :exec
INSERT INTO order_items (
    business_id, outlet_id, order_id, service_id, perfume_id,
    service_name_snapshot, service_unit_snapshot, unit_price_amount_snapshot,
    perfume_name_snapshot, quantity, line_total_amount, notes
)
VALUES (
    sqlc.arg(business_id), sqlc.arg(outlet_id), sqlc.arg(order_id), sqlc.arg(service_id), sqlc.arg(perfume_id),
    sqlc.arg(service_name_snapshot), sqlc.arg(service_unit_snapshot), sqlc.arg(unit_price_amount_snapshot),
    sqlc.arg(perfume_name_snapshot), sqlc.arg(quantity), sqlc.arg(line_total_amount), sqlc.arg(notes)
);

-- name: CreateInitialOrderStatus :exec
INSERT INTO order_status_history (business_id, outlet_id, order_id, from_status, to_status, changed_by)
VALUES (sqlc.arg(business_id), sqlc.arg(outlet_id), sqlc.arg(order_id), NULL, 'RECEIVED', sqlc.arg(changed_by));

-- name: InsertOrderAuditLog :exec
INSERT INTO audit_logs (business_id, outlet_id, actor_user_id, action, entity_type, entity_id, new_values, ip_address, user_agent)
VALUES (sqlc.arg(business_id), sqlc.arg(outlet_id), sqlc.arg(actor_user_id), 'ORDER_CREATED', 'order', sqlc.arg(order_id),
        sqlc.arg(new_values), sqlc.arg(ip_address), sqlc.arg(user_agent));

-- name: GetOrder :one
SELECT id, business_id, outlet_id, customer_id, invoice_number, status, payment_status,
       total_amount, notes, received_at, due_at, created_by, created_at, updated_at
FROM orders
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(order_id)
  AND (sqlc.arg(is_admin)::boolean OR outlet_id = ANY(sqlc.arg(outlet_ids)::bigint[]));

-- name: ListOrders :many
SELECT id, business_id, outlet_id, customer_id, invoice_number, status, payment_status,
       total_amount, notes, received_at, due_at, created_by, created_at, updated_at
FROM orders
WHERE business_id = sqlc.arg(business_id)
  AND (sqlc.arg(is_admin)::boolean OR outlet_id = ANY(sqlc.arg(outlet_ids)::bigint[]))
  AND (sqlc.narg(outlet_id)::bigint IS NULL OR outlet_id = sqlc.narg(outlet_id)::bigint)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text)
ORDER BY created_at DESC, id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountOrders :one
SELECT count(*)
FROM orders
WHERE business_id = sqlc.arg(business_id)
  AND (sqlc.arg(is_admin)::boolean OR outlet_id = ANY(sqlc.arg(outlet_ids)::bigint[]))
  AND (sqlc.narg(outlet_id)::bigint IS NULL OR outlet_id = sqlc.narg(outlet_id)::bigint)
  AND (sqlc.narg(status)::text IS NULL OR status = sqlc.narg(status)::text);

-- name: ListOrderItems :many
SELECT id, service_id, perfume_id, service_name_snapshot, service_unit_snapshot,
       unit_price_amount_snapshot, perfume_name_snapshot, quantity::text AS quantity,
       line_total_amount, notes
FROM order_items
WHERE business_id = sqlc.arg(business_id) AND outlet_id = sqlc.arg(outlet_id) AND order_id = sqlc.arg(order_id)
ORDER BY id;
