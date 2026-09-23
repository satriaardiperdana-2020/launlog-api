-- name: GetAuthorizedReceiptOrder :one
SELECT o.id, o.business_id, o.outlet_id, o.customer_id, o.invoice_number,
       o.status, o.payment_status, o.total_amount, o.received_at, o.due_at,
       o.customer_name_snapshot, o.customer_phone_snapshot, o.receipt_qr_id,
       outlet.code AS outlet_code, outlet.name AS outlet_name,
       outlet.phone AS outlet_phone, outlet.address AS outlet_address,
       template.id AS receipt_template_id, template.name AS receipt_template_name,
       template.header_text AS receipt_header_text, template.footer_text AS receipt_footer_text,
       template.whatsapp_message_template, template.paper_width_mm
FROM orders o
JOIN outlets outlet ON outlet.business_id=o.business_id AND outlet.id=o.outlet_id AND outlet.is_active
JOIN businesses b ON b.id=o.business_id AND b.is_active
JOIN users actor ON actor.business_id=o.business_id AND actor.id=sqlc.arg(user_id)
    AND actor.is_active
    AND actor.role=CASE WHEN sqlc.arg(is_admin)::boolean THEN 'ADMIN' ELSE 'LAUNDRY_STAFF' END
LEFT JOIN receipt_templates template ON template.business_id=o.business_id
    AND template.outlet_id=o.outlet_id AND template.is_default AND template.deleted_at IS NULL
WHERE o.business_id=sqlc.arg(business_id)
  AND (sqlc.narg(outlet_id)::bigint IS NULL OR o.outlet_id=sqlc.narg(outlet_id)::bigint)
  AND (
      (sqlc.narg(order_id)::bigint IS NOT NULL AND o.id=sqlc.narg(order_id)::bigint)
      OR (sqlc.narg(qr_id)::text IS NOT NULL AND o.receipt_qr_id=sqlc.narg(qr_id)::text)
  )
  AND (sqlc.arg(is_admin)::boolean OR EXISTS (
      SELECT 1 FROM user_outlets assignment
      WHERE assignment.business_id=o.business_id
        AND assignment.outlet_id=o.outlet_id
        AND assignment.user_id=actor.id
  ));

-- name: GetReceiptConfirmedPaymentTotal :one
SELECT coalesce(sum(amount),0)::bigint
FROM payments
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id)
  AND order_id=sqlc.arg(order_id) AND status='CONFIRMED';

-- name: LockReceiptTemplateOutlet :one
SELECT outlet.id
FROM outlets outlet
JOIN businesses b ON b.id=outlet.business_id AND b.is_active
JOIN users actor ON actor.business_id=outlet.business_id AND actor.id=sqlc.arg(user_id)
    AND actor.is_active
    AND actor.role=CASE WHEN sqlc.arg(is_admin)::boolean THEN 'ADMIN' ELSE 'LAUNDRY_STAFF' END
WHERE outlet.business_id=sqlc.arg(business_id) AND outlet.id=sqlc.arg(outlet_id)
  AND outlet.is_active
  AND (sqlc.arg(is_admin)::boolean OR EXISTS (
      SELECT 1 FROM user_outlets assignment
      WHERE assignment.business_id=outlet.business_id
        AND assignment.outlet_id=outlet.id AND assignment.user_id=actor.id
  ))
FOR UPDATE OF outlet;

-- name: GetAuthorizedReceiptOutlet :one
SELECT outlet.id
FROM outlets outlet
JOIN businesses b ON b.id=outlet.business_id AND b.is_active
JOIN users actor ON actor.business_id=outlet.business_id AND actor.id=sqlc.arg(user_id)
    AND actor.is_active
    AND actor.role=CASE WHEN sqlc.arg(is_admin)::boolean THEN 'ADMIN' ELSE 'LAUNDRY_STAFF' END
WHERE outlet.business_id=sqlc.arg(business_id) AND outlet.id=sqlc.arg(outlet_id)
  AND outlet.is_active
  AND (sqlc.arg(is_admin)::boolean OR EXISTS (
      SELECT 1 FROM user_outlets assignment
      WHERE assignment.business_id=outlet.business_id
        AND assignment.outlet_id=outlet.id AND assignment.user_id=actor.id
  ));

-- name: ListReceiptTemplates :many
SELECT id, business_id, outlet_id, name, header_text, footer_text,
       whatsapp_message_template, paper_width_mm, is_default, created_at, updated_at
FROM receipt_templates
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id)
  AND deleted_at IS NULL
ORDER BY is_default DESC, lower(name), id;

-- name: GetReceiptTemplateForUpdate :one
SELECT id, business_id, outlet_id, name, header_text, footer_text,
       whatsapp_message_template, paper_width_mm, is_default, created_at, updated_at
FROM receipt_templates
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id)
  AND id=sqlc.arg(template_id) AND deleted_at IS NULL
FOR UPDATE;

-- name: HasActiveDefaultReceiptTemplate :one
SELECT EXISTS (
    SELECT 1 FROM receipt_templates
    WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id)
      AND is_default AND deleted_at IS NULL
);

-- name: ClearActiveDefaultReceiptTemplates :exec
UPDATE receipt_templates SET is_default=FALSE, updated_at=now()
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id)
  AND is_default AND deleted_at IS NULL;

-- name: CreateReceiptTemplate :one
INSERT INTO receipt_templates (
    business_id,outlet_id,name,header_text,footer_text,
    whatsapp_message_template,paper_width_mm,is_default
)
VALUES (
    sqlc.arg(business_id),sqlc.arg(outlet_id),sqlc.arg(name),sqlc.arg(header_text),sqlc.arg(footer_text),
    sqlc.arg(whatsapp_message_template),sqlc.arg(paper_width_mm),sqlc.arg(is_default)
)
RETURNING id,business_id,outlet_id,name,header_text,footer_text,
          whatsapp_message_template,paper_width_mm,is_default,created_at,updated_at;

-- name: UpdateReceiptTemplate :one
UPDATE receipt_templates SET
    name=sqlc.arg(name), header_text=sqlc.arg(header_text), footer_text=sqlc.arg(footer_text),
    whatsapp_message_template=sqlc.arg(whatsapp_message_template),
    paper_width_mm=sqlc.arg(paper_width_mm), is_default=sqlc.arg(is_default), updated_at=now()
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id)
  AND id=sqlc.arg(template_id) AND deleted_at IS NULL
RETURNING id,business_id,outlet_id,name,header_text,footer_text,
          whatsapp_message_template,paper_width_mm,is_default,created_at,updated_at;

-- name: SoftDeleteReceiptTemplate :one
UPDATE receipt_templates SET deleted_at=now(),is_default=FALSE,updated_at=now()
WHERE business_id=sqlc.arg(business_id) AND outlet_id=sqlc.arg(outlet_id)
  AND id=sqlc.arg(template_id) AND deleted_at IS NULL
RETURNING id,business_id,outlet_id,name,header_text,footer_text,
          whatsapp_message_template,paper_width_mm,is_default,created_at,updated_at;

-- name: SetNextReceiptTemplateDefault :exec
WITH candidate AS (
    SELECT template.id FROM receipt_templates template
    WHERE template.business_id=sqlc.arg(business_id) AND template.outlet_id=sqlc.arg(outlet_id)
      AND template.id<>sqlc.arg(exclude_template_id) AND template.deleted_at IS NULL
    ORDER BY template.id LIMIT 1
)
UPDATE receipt_templates target SET is_default=TRUE,updated_at=now()
FROM candidate
WHERE target.business_id=sqlc.arg(business_id) AND target.outlet_id=sqlc.arg(outlet_id)
  AND target.id=candidate.id;

-- name: InsertReceiptTemplateAuditLog :exec
INSERT INTO audit_logs (business_id,outlet_id,actor_user_id,action,entity_type,entity_id,
                        old_values,new_values,ip_address,user_agent)
VALUES (sqlc.arg(business_id),sqlc.arg(outlet_id),sqlc.arg(actor_user_id),sqlc.arg(action),
        'receipt_template',sqlc.arg(template_id),sqlc.arg(old_values),sqlc.arg(new_values),
        sqlc.arg(ip_address),sqlc.arg(user_agent));
