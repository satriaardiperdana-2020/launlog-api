-- name: ListPerfumes :many
SELECT id, business_id, name, description, is_active, created_at, updated_at
FROM perfumes
WHERE business_id = sqlc.arg(business_id) AND deleted_at IS NULL
  AND (sqlc.arg(search)::text = '' OR name ILIKE '%' || sqlc.arg(search)::text || '%'
       OR COALESCE(description, '') ILIKE '%' || sqlc.arg(search)::text || '%')
  AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active)::boolean)
ORDER BY lower(name), id
LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);

-- name: CountPerfumes :one
SELECT count(*)
FROM perfumes
WHERE business_id = sqlc.arg(business_id) AND deleted_at IS NULL
  AND (sqlc.arg(search)::text = '' OR name ILIKE '%' || sqlc.arg(search)::text || '%'
       OR COALESCE(description, '') ILIKE '%' || sqlc.arg(search)::text || '%')
  AND (sqlc.narg(is_active)::boolean IS NULL OR is_active = sqlc.narg(is_active)::boolean);

-- name: GetPerfume :one
SELECT id, business_id, name, description, is_active, created_at, updated_at
FROM perfumes
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(perfume_id) AND deleted_at IS NULL;

-- name: GetPerfumeForUpdate :one
SELECT id, business_id, name, description, is_active, created_at, updated_at
FROM perfumes
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(perfume_id) AND deleted_at IS NULL
FOR UPDATE;

-- name: CreatePerfume :one
INSERT INTO perfumes (business_id, name, description)
VALUES (sqlc.arg(business_id), sqlc.arg(name), sqlc.arg(description))
RETURNING id, business_id, name, description, is_active, created_at, updated_at;

-- name: UpdatePerfume :one
UPDATE perfumes
SET name = sqlc.arg(name), description = sqlc.arg(description), is_active = sqlc.arg(is_active), updated_at = now()
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(perfume_id) AND deleted_at IS NULL
RETURNING id, business_id, name, description, is_active, created_at, updated_at;

-- name: SoftDeletePerfume :one
UPDATE perfumes
SET is_active = FALSE, deleted_at = now(), updated_at = now()
WHERE business_id = sqlc.arg(business_id) AND id = sqlc.arg(perfume_id) AND deleted_at IS NULL
RETURNING id;
