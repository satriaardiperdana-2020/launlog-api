-- name: GetPaymentByIdempotencyKey :one
SELECT id, business_id, outlet_id, order_id, amount, method, status,
       external_reference, notes, confirmed_at, voided_at, voided_by, void_reason,
       created_by, created_at, request_hash
FROM payments
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id)
  AND order_id=sqlc.arg(order_id) AND idempotency_key=sqlc.arg(idempotency_key);

-- name: GetConfirmedPaymentTotal :one
SELECT COALESCE(sum(amount), 0)::bigint AS paid_amount
FROM payments
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id)
  AND order_id=sqlc.arg(order_id) AND status='CONFIRMED';

-- name: CountOrderPaymentRefunds :one
SELECT count(*) FROM payment_refunds
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id) AND order_id=sqlc.arg(order_id);

-- name: CreateConfirmedPayment :one
INSERT INTO payments (
    business_id, outlet_id, order_id, amount, method, status, external_reference,
    notes, confirmed_at, created_by, idempotency_key, request_hash
)
VALUES (
    sqlc.arg(business_id), sqlc.arg(outlet_id), sqlc.arg(order_id), sqlc.arg(amount),
    sqlc.arg(method), 'CONFIRMED', sqlc.arg(external_reference), sqlc.arg(notes),
    now(), sqlc.arg(created_by), sqlc.arg(idempotency_key), sqlc.arg(request_hash)
)
RETURNING id, business_id, outlet_id, order_id, amount, method, status,
          external_reference, notes, confirmed_at, created_by, created_at;

-- name: UpdateOrderPaymentStatus :one
UPDATE orders
SET payment_status=sqlc.arg(payment_status), updated_at=now()
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id) AND id=sqlc.arg(order_id)
RETURNING payment_status, updated_at;

-- name: ListOrderPayments :many
SELECT id, amount, method, status, external_reference, notes, confirmed_at, voided_at,
       voided_by, void_reason, created_by, created_at
FROM payments
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id) AND order_id=sqlc.arg(order_id)
ORDER BY created_at, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountOrderPayments :one
SELECT count(*) FROM payments
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id) AND order_id=sqlc.arg(order_id);

-- name: GetOrderPaymentForUpdate :one
SELECT id, amount, method, status, external_reference, notes, confirmed_at, voided_at,
       voided_by, void_reason, created_by, created_at
FROM payments
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id)
  AND order_id=sqlc.arg(order_id) AND id=sqlc.arg(payment_id)
FOR UPDATE;

-- name: VoidConfirmedPayment :one
UPDATE payments SET status='VOIDED', voided_at=now(), voided_by=sqlc.arg(voided_by), void_reason=sqlc.arg(void_reason)
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id)
  AND order_id=sqlc.arg(order_id) AND id=sqlc.arg(payment_id) AND status='CONFIRMED'
RETURNING id, amount, method, status, external_reference, notes, confirmed_at, voided_at,
          voided_by, void_reason, created_by, created_at;

-- name: InsertPaymentAuditLog :exec
INSERT INTO audit_logs (business_id, outlet_id, actor_user_id, action, entity_type, entity_id, old_values, new_values, ip_address, user_agent)
VALUES (sqlc.arg(business_id), sqlc.arg(outlet_id), sqlc.arg(actor_user_id), sqlc.arg(action), 'payment',
        sqlc.arg(payment_id), sqlc.arg(old_values), sqlc.arg(new_values), sqlc.arg(ip_address), sqlc.arg(user_agent));
