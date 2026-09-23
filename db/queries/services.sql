-- name: ListServices :many
SELECT id, business_id, name, description, unit, unit_price_amount,
       estimated_duration_minutes, is_active, created_at, updated_at
FROM services
WHERE business_id = sqlc.arg(business_id) AND deleted_at IS NULL
  AND (sqlc.arg(search)::text = '' OR name ILIKE '%' || sqlc.arg(search)::text || '%'
       OR COALESCE(description, '') ILIKE '%' || sqlc.arg(search)::text || '%')
  AND (sqlc.narg(unit)::text IS NULL OR unit = sqlc.narg(unit)::text)
  AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active)::boolean)
ORDER BY lower(name), unit, id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountServices :one
SELECT count(*)
FROM services
WHERE business_id = sqlc.arg(business_id) AND deleted_at IS NULL
  AND (sqlc.arg(search)::text = '' OR name ILIKE '%' || sqlc.arg(search)::text || '%'
       OR COALESCE(description, '') ILIKE '%' || sqlc.arg(search)::text || '%')
  AND (sqlc.narg(unit)::text IS NULL OR unit = sqlc.narg(unit)::text)
  AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active)::boolean);

-- name: GetService :one
SELECT id, business_id, name, description, unit, unit_price_amount,
       estimated_duration_minutes, is_active, created_at, updated_at
FROM services
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(service_id) AND deleted_at IS NULL;

-- name: GetServiceForUpdate :one
SELECT id, business_id, name, description, unit, unit_price_amount,
       estimated_duration_minutes, is_active, created_at, updated_at
FROM services
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(service_id) AND deleted_at IS NULL
FOR UPDATE;

-- name: CreateService :one
INSERT INTO services (business_id, name, description, unit, unit_price_amount, estimated_duration_minutes)
VALUES (sqlc.arg(business_id), sqlc.arg(name), sqlc.arg(description), sqlc.arg(unit),
        sqlc.arg(unit_price_amount), sqlc.arg(estimated_duration_minutes))
RETURNING id, business_id, name, description, unit, unit_price_amount,
          estimated_duration_minutes, is_active, created_at, updated_at;

-- name: UpdateService :one
UPDATE services
SET name = sqlc.arg(name), description = sqlc.arg(description), unit = sqlc.arg(unit),
    unit_price_amount = sqlc.arg(unit_price_amount),
    estimated_duration_minutes = sqlc.arg(estimated_duration_minutes),
    is_active = sqlc.arg(is_active), updated_at = now()
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(service_id) AND deleted_at IS NULL
RETURNING id, business_id, name, description, unit, unit_price_amount,
          estimated_duration_minutes, is_active, created_at, updated_at;

-- name: SoftDeleteService :one
UPDATE services
SET is_active = FALSE, deleted_at = now(), updated_at = now()
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(service_id) AND deleted_at IS NULL
RETURNING id;
