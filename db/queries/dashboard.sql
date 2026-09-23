-- name: GetOutletDashboard :one
WITH outlet_scope AS MATERIALIZED (
    SELECT o.id, o.business_id, o.code, o.name, o.phone, o.address, o.is_active, o.created_at, o.updated_at,
           b.timezone,
           (now() AT TIME ZONE b.timezone)::date AS business_date
    FROM outlets o
    JOIN businesses b ON b.id=o.business_id AND b.is_active
    JOIN users actor ON actor.business_id=o.business_id
        AND actor.id=sqlc.arg(user_id) AND actor.is_active
        AND actor.role=CASE WHEN sqlc.arg(is_admin)::boolean THEN 'ADMIN' ELSE 'LAUNDRY_STAFF' END
    WHERE o.business_id=sqlc.arg(business_id)
      AND o.id=sqlc.arg(outlet_id)
      AND o.is_active
      AND (sqlc.arg(is_admin)::boolean OR EXISTS (
          SELECT 1 FROM user_outlets assignment
          WHERE assignment.business_id=o.business_id
            AND assignment.outlet_id=o.id
            AND assignment.user_id=actor.id
      ))
), local_day AS (
    SELECT outlet_scope.*,
           business_date::timestamp AT TIME ZONE timezone AS day_start,
           (business_date + 1)::timestamp AT TIME ZONE timezone AS day_end
    FROM outlet_scope
), payment_totals AS (
    SELECT p.business_id, p.outlet_id,
           count(*)::bigint AS confirmed_payment_count,
           COALESCE(sum(p.amount), 0)::bigint AS confirmed_payment_amount
    FROM payments p
    JOIN local_day d ON d.business_id=p.business_id AND d.id=p.outlet_id
    WHERE p.status='CONFIRMED'
      AND p.confirmed_at >= d.day_start
      AND p.confirmed_at < d.day_end
    GROUP BY p.business_id, p.outlet_id
), expense_totals AS (
    SELECT e.business_id, e.outlet_id,
           count(*)::bigint AS expense_count,
           COALESCE(sum(e.amount), 0)::bigint AS expense_amount
    FROM expenses e
    JOIN local_day d ON d.business_id=e.business_id AND d.id=e.outlet_id
    WHERE e.expense_at >= d.day_start
      AND e.expense_at < d.day_end
    GROUP BY e.business_id, e.outlet_id
)
SELECT d.id AS outlet_id, d.business_id, d.code, d.name, d.phone, d.address, d.is_active, d.created_at, d.updated_at,
       d.timezone, d.business_date,
       COALESCE(p.confirmed_payment_count, 0)::bigint AS confirmed_payment_count,
       COALESCE(p.confirmed_payment_amount, 0)::bigint AS confirmed_payment_amount,
       COALESCE(e.expense_count, 0)::bigint AS expense_count,
       COALESCE(e.expense_amount, 0)::bigint AS expense_amount
FROM local_day d
LEFT JOIN payment_totals p ON p.business_id=d.business_id AND p.outlet_id=d.id
LEFT JOIN expense_totals e ON e.business_id=d.business_id AND e.outlet_id=d.id;
