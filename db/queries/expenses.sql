-- name: ListExpenseCategories :many
SELECT id, business_id, name, description, is_active, created_at, updated_at
FROM expense_categories
WHERE business_id = sqlc.arg(business_id)
  AND (sqlc.arg(search)::text = '' OR name ILIKE '%' || sqlc.arg(search)::text || '%')
  AND (sqlc.arg(include_inactive)::boolean OR is_active)
ORDER BY lower(name), id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountExpenseCategories :one
SELECT count(*) FROM expense_categories
WHERE business_id = sqlc.arg(business_id)
  AND (sqlc.arg(search)::text = '' OR name ILIKE '%' || sqlc.arg(search)::text || '%')
  AND (sqlc.arg(include_inactive)::boolean OR is_active);

-- name: GetExpenseCategory :one
SELECT id, business_id, name, description, is_active, created_at, updated_at
FROM expense_categories WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(category_id);

-- name: GetExpenseCategoryForUpdate :one
SELECT id, business_id, name, description, is_active, created_at, updated_at
FROM expense_categories WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(category_id)
FOR UPDATE;

-- name: CreateExpenseCategory :one
INSERT INTO expense_categories (business_id, name, description)
VALUES (sqlc.arg(business_id), sqlc.arg(name), sqlc.arg(description))
RETURNING id, business_id, name, description, is_active, created_at, updated_at;

-- name: UpdateExpenseCategory :one
UPDATE expense_categories SET name=sqlc.arg(name), description=sqlc.arg(description),
    is_active=sqlc.arg(is_active), updated_at=now()
WHERE business_id=sqlc.arg(business_id) AND id=sqlc.arg(category_id)
RETURNING id, business_id, name, description, is_active, created_at, updated_at;

-- name: ListExpenses :many
SELECT expenses.id, expenses.business_id, expenses.outlet_id, expenses.category_id,
       expenses.amount, expenses.description, expenses.expense_at,
       expenses.receipt_reference, expenses.created_by, expenses.created_at, expenses.updated_at, expenses.version
FROM expenses
JOIN outlets o ON o.business_id=expenses.business_id AND o.id=expenses.outlet_id AND o.is_active
WHERE expenses.business_id=sqlc.arg(business_id)
  AND (sqlc.arg(is_admin)::boolean OR expenses.outlet_id=ANY(sqlc.arg(outlet_ids)::bigint[]))
  AND (sqlc.narg(outlet_id)::bigint IS NULL OR expenses.outlet_id=sqlc.narg(outlet_id))
  AND expenses.expense_at >= (sqlc.arg(date_from)::date::timestamp AT TIME ZONE 'Asia/Jakarta')
  AND expenses.expense_at < ((sqlc.arg(date_to)::date + 1)::timestamp AT TIME ZONE 'Asia/Jakarta')
ORDER BY expenses.expense_at DESC, expenses.id DESC
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountExpenses :one
SELECT count(*) FROM expenses
JOIN outlets o ON o.business_id=expenses.business_id AND o.id=expenses.outlet_id AND o.is_active
WHERE expenses.business_id=sqlc.arg(business_id)
  AND (sqlc.arg(is_admin)::boolean OR expenses.outlet_id=ANY(sqlc.arg(outlet_ids)::bigint[]))
  AND (sqlc.narg(outlet_id)::bigint IS NULL OR expenses.outlet_id=sqlc.narg(outlet_id))
  AND expenses.expense_at >= (sqlc.arg(date_from)::date::timestamp AT TIME ZONE 'Asia/Jakarta')
  AND expenses.expense_at < ((sqlc.arg(date_to)::date + 1)::timestamp AT TIME ZONE 'Asia/Jakarta');

-- name: GetExpense :one
SELECT e.id, e.business_id, e.outlet_id, e.category_id, e.amount, e.description, e.expense_at,
       e.receipt_reference, e.created_by, e.created_at, e.updated_at, e.version
FROM expenses e
JOIN outlets o ON o.business_id=e.business_id AND o.id=e.outlet_id AND o.is_active
WHERE e.business_id=sqlc.arg(business_id) AND e.id=sqlc.arg(expense_id)
  AND (sqlc.arg(is_admin)::boolean OR EXISTS (
      SELECT 1 FROM user_outlets uo WHERE uo.business_id=e.business_id
        AND uo.outlet_id=e.outlet_id AND uo.user_id=sqlc.arg(user_id)))
;

-- name: GetExpenseForUpdate :one
SELECT e.id, e.business_id, e.outlet_id, e.category_id, e.amount, e.description, e.expense_at,
       e.receipt_reference, e.created_by, e.created_at, e.updated_at, e.version
FROM expenses e
JOIN outlets o ON o.business_id=e.business_id AND o.id=e.outlet_id AND o.is_active
JOIN users u ON u.business_id=e.business_id AND u.id=sqlc.arg(user_id) AND u.is_active
WHERE e.business_id=sqlc.arg(business_id) AND e.id=sqlc.arg(expense_id)
  AND (sqlc.arg(is_admin)::boolean OR EXISTS (
      SELECT 1 FROM user_outlets uo WHERE uo.business_id=e.business_id
        AND uo.outlet_id=e.outlet_id AND uo.user_id=sqlc.arg(user_id)))
FOR UPDATE OF e, o, u;

-- name: GetActiveExpenseCategory :one
SELECT id FROM expense_categories
WHERE business_id=sqlc.arg(business_id) AND id=sqlc.arg(category_id) AND is_active
FOR SHARE;

-- name: LockActiveExpenseOutlet :one
SELECT o.id FROM outlets o
JOIN users u ON u.business_id=o.business_id AND u.id=sqlc.arg(user_id) AND u.is_active
WHERE o.business_id=sqlc.arg(business_id) AND o.id=sqlc.arg(outlet_id) AND o.is_active
  AND (sqlc.arg(is_admin)::boolean OR EXISTS (
      SELECT 1 FROM user_outlets uo WHERE uo.business_id=o.business_id AND uo.outlet_id=o.id AND uo.user_id=u.id))
FOR SHARE OF o, u;

-- name: LockExpenseOutletAssignment :one
SELECT user_id FROM user_outlets
WHERE business_id=sqlc.arg(business_id) AND user_id=sqlc.arg(user_id) AND outlet_id=sqlc.arg(outlet_id)
FOR SHARE;

-- name: CreateExpense :one
INSERT INTO expenses (business_id,outlet_id,category_id,amount,description,expense_at,receipt_reference,created_by)
SELECT sqlc.arg(business_id), o.id, sqlc.arg(category_id), sqlc.arg(amount), sqlc.arg(description),
       sqlc.arg(expense_at), sqlc.arg(receipt_reference), sqlc.arg(user_id)
FROM outlets o
JOIN users u ON u.business_id=o.business_id AND u.id=sqlc.arg(user_id) AND u.is_active
JOIN expense_categories c ON c.business_id=o.business_id AND c.id=sqlc.arg(category_id) AND c.is_active
WHERE o.business_id=sqlc.arg(business_id) AND o.id=sqlc.arg(outlet_id) AND o.is_active
  AND (sqlc.arg(is_admin)::boolean OR EXISTS (
      SELECT 1 FROM user_outlets uo WHERE uo.business_id=o.business_id AND uo.outlet_id=o.id AND uo.user_id=u.id))
RETURNING id,business_id,outlet_id,category_id,amount,description,expense_at,receipt_reference,created_by,created_at,updated_at,version;

-- name: UpdateExpense :one
UPDATE expenses e SET category_id=sqlc.arg(category_id), amount=sqlc.arg(amount),
    description=sqlc.arg(description), expense_at=sqlc.arg(expense_at),
    receipt_reference=sqlc.arg(receipt_reference), updated_at=now(), version=version+1
WHERE e.business_id=sqlc.arg(business_id) AND e.id=sqlc.arg(expense_id)
  AND e.version=sqlc.arg(expected_version)
  AND EXISTS (SELECT 1 FROM outlets o WHERE o.business_id=e.business_id AND o.id=e.outlet_id AND o.is_active)
  AND EXISTS (SELECT 1 FROM users u WHERE u.business_id=e.business_id AND u.id=sqlc.arg(user_id) AND u.is_active)
  AND (sqlc.arg(is_admin)::boolean OR EXISTS (
      SELECT 1 FROM user_outlets uo WHERE uo.business_id=e.business_id AND uo.outlet_id=e.outlet_id AND uo.user_id=sqlc.arg(user_id)))
  AND EXISTS (SELECT 1 FROM expense_categories c WHERE c.business_id=e.business_id AND c.id=sqlc.arg(category_id) AND c.is_active)
RETURNING id,business_id,outlet_id,category_id,amount,description,expense_at,receipt_reference,created_by,created_at,updated_at,version;

-- name: InsertExpenseAuditLog :exec
INSERT INTO audit_logs (business_id,outlet_id,actor_user_id,action,entity_type,entity_id,old_values,new_values,ip_address,user_agent)
VALUES (sqlc.arg(business_id),sqlc.arg(outlet_id),sqlc.arg(actor_user_id),sqlc.arg(action),'expense',
        sqlc.arg(entity_id),sqlc.arg(old_values),sqlc.arg(new_values),sqlc.arg(ip_address),sqlc.arg(user_agent));

-- name: InsertExpenseCategoryAuditLog :exec
INSERT INTO audit_logs (business_id,actor_user_id,action,entity_type,entity_id,old_values,new_values,ip_address,user_agent)
VALUES (sqlc.arg(business_id),sqlc.arg(actor_user_id),sqlc.arg(action),'expense_category',
        sqlc.arg(entity_id),sqlc.arg(old_values),sqlc.arg(new_values),sqlc.arg(ip_address),sqlc.arg(user_agent));
