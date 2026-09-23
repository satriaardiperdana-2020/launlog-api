-- name: GetUserForLogin :one
SELECT id, business_id, email, full_name, password_hash, role, is_active
FROM users
WHERE lower(email) = lower($1);

-- name: GetActiveUser :one
SELECT id, business_id, email, full_name, role, is_active
FROM users
WHERE id = $1 AND business_id = $2 AND is_active = TRUE;

-- name: ListUserOutletIDs :many
SELECT outlet_id
FROM user_outlets
WHERE business_id = $1 AND user_id = $2
ORDER BY outlet_id;

-- name: ListUserPermissionCodes :many
SELECT permission_code
FROM user_permissions
WHERE business_id = $1 AND user_id = $2
ORDER BY permission_code;

-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (business_id, user_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4)
RETURNING id, business_id, user_id, token_hash, expires_at, revoked_at;

-- name: GetRefreshTokenForUpdate :one
SELECT id, business_id, user_id, token_hash, expires_at, revoked_at
FROM refresh_tokens
WHERE token_hash = $1
FOR UPDATE;

-- name: RevokeRefreshToken :execrows
UPDATE refresh_tokens
SET revoked_at = now()
WHERE id = $1 AND business_id = $2 AND user_id = $3 AND revoked_at IS NULL;

-- name: GetActiveRefreshSession :one
SELECT id, business_id, user_id, expires_at, revoked_at
FROM refresh_tokens
WHERE id = $1
  AND business_id = $2
  AND user_id = $3
  AND revoked_at IS NULL
  AND expires_at > now();
