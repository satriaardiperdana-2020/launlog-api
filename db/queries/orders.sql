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

-- name: InsertOrderLifecycleAuditLog :exec
INSERT INTO audit_logs (business_id, outlet_id, actor_user_id, action, entity_type, entity_id, old_values, new_values, ip_address, user_agent)
VALUES (sqlc.arg(business_id), sqlc.arg(outlet_id), sqlc.arg(actor_user_id), sqlc.arg(action), 'order',
        sqlc.arg(order_id), sqlc.arg(old_values), sqlc.arg(new_values), sqlc.arg(ip_address), sqlc.arg(user_agent));

-- name: GetOrder :one
SELECT id, business_id, outlet_id, customer_id, invoice_number, status, payment_status,
       total_amount, notes, received_at, due_at, created_by, created_at, updated_at
FROM orders
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(order_id)
  AND (sqlc.arg(is_admin)::boolean OR outlet_id = ANY(sqlc.arg(outlet_ids)::bigint[]));

-- name: GetOrderForUpdate :one
SELECT o.id, o.business_id, o.outlet_id, o.status, o.payment_status, o.total_amount, o.completed_at,
       o.cancelled_at, o.cancelled_by, o.cancellation_reason, o.deleted_at
FROM orders AS o
JOIN outlets AS outlet ON outlet.business_id=o.business_id AND outlet.id=o.outlet_id AND outlet.is_active
JOIN users AS current_actor ON current_actor.business_id=o.business_id
  AND current_actor.id=sqlc.arg(user_id) AND current_actor.is_active
  AND current_actor.role=CASE WHEN sqlc.arg(is_admin)::boolean THEN 'ADMIN' ELSE 'LAUNDRY_STAFF' END
WHERE o.business_id=sqlc.arg(business_id) AND o.id=sqlc.arg(order_id)
  AND (sqlc.arg(is_admin)::boolean OR EXISTS (
      SELECT 1 FROM user_outlets AS assignment
      WHERE assignment.business_id=o.business_id AND assignment.outlet_id=o.outlet_id
        AND assignment.user_id=sqlc.arg(user_id)
  ))
FOR UPDATE OF o, outlet, current_actor;

-- name: LockCurrentOrderOutletAssignment :one
SELECT user_id FROM user_outlets
WHERE business_id=sqlc.arg(business_id) AND user_id=sqlc.arg(user_id) AND outlet_id=sqlc.arg(outlet_id)
FOR SHARE;

-- name: TransitionOrderStatus :one
UPDATE orders SET status=sqlc.arg(to_status),
    completed_at=CASE WHEN sqlc.arg(to_status)::text='COMPLETED' THEN now() ELSE NULL END,
    updated_at=now()
WHERE business_id=sqlc.arg(business_id) AND id=sqlc.arg(order_id)
RETURNING id, business_id, outlet_id, status, payment_status, completed_at, updated_at;

-- name: CancelOrder :one
UPDATE orders SET status='CANCELLED', cancelled_at=now(), cancelled_by=sqlc.arg(actor_user_id),
    cancellation_reason=sqlc.arg(reason), deleted_at=now(), updated_at=now()
WHERE business_id=sqlc.arg(business_id) AND id=sqlc.arg(order_id)
RETURNING id, business_id, outlet_id, status, payment_status, cancelled_at, cancelled_by,
          cancellation_reason, deleted_at, updated_at;

-- name: InsertOrderStatusHistory :exec
INSERT INTO order_status_history (business_id, outlet_id, order_id, from_status, to_status, changed_by, notes)
VALUES (sqlc.arg(business_id), sqlc.arg(outlet_id), sqlc.arg(order_id), sqlc.arg(from_status),
        sqlc.arg(to_status), sqlc.arg(changed_by), sqlc.arg(notes));

-- name: ListOrderStatusHistory :many
SELECT h.id, h.from_status, h.to_status, h.changed_by, u.full_name AS changed_by_name,
       h.notes, h.changed_at
FROM order_status_history AS h
JOIN users AS u ON u.business_id=h.business_id AND u.id=h.changed_by
WHERE h.business_id=sqlc.arg(business_id) AND h.outlet_id=sqlc.arg(outlet_id)
  AND h.order_id=sqlc.arg(order_id)
ORDER BY h.changed_at, h.id;

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
