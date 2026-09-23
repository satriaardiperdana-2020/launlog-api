-- name: GetUserForLogin :one
SELECT users.id, users.business_id, users.email, users.full_name, users.password_hash, users.role, users.is_active
FROM users
JOIN businesses b ON b.id = users.business_id AND b.is_active = TRUE
WHERE lower(email) = lower($1);

-- name: GetActiveUser :one
SELECT users.id, users.business_id, users.email, users.full_name, users.role, users.is_active
FROM users
JOIN businesses b ON b.id = users.business_id AND b.is_active = TRUE
WHERE users.id = $1 AND users.business_id = $2 AND users.is_active = TRUE;

-- name: ListUserOutletIDs :many
SELECT outlet_id
FROM user_outlets uo
JOIN outlets o ON o.id = uo.outlet_id AND o.business_id = uo.business_id AND o.is_active = TRUE
WHERE uo.business_id = $1 AND uo.user_id = $2
ORDER BY outlet_id;

-- name: ListUserPermissionCodes :many
SELECT permission_code
FROM user_permissions
WHERE business_id = $1 AND user_id = $2
ORDER BY permission_code;

-- name: CreateRefreshToken :one
INSERT INTO refresh_tokens (business_id, user_id, family_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, business_id, user_id, family_id, token_hash, expires_at, revoked_at;

-- name: CreateSessionFamily :one
INSERT INTO session_families (business_id, user_id)
VALUES ($1, $2)
RETURNING id;

-- name: FindRefreshTokenFamily :one
SELECT family_id, business_id, user_id
FROM refresh_tokens
WHERE token_hash = $1;

-- name: LockSessionFamily :one
SELECT id, business_id, user_id, revoked_at
FROM session_families
WHERE id = $1 AND business_id = $2 AND user_id = $3
FOR UPDATE;

-- name: RevokeSessionFamily :exec
UPDATE session_families SET revoked_at = now()
WHERE id = $1 AND business_id = $2 AND user_id = $3 AND revoked_at IS NULL;

-- name: GetRefreshTokenForUpdate :one
SELECT id, business_id, user_id, family_id, token_hash, expires_at, revoked_at
FROM refresh_tokens
WHERE token_hash = $1;

-- name: GetRefreshTokenByID :one
SELECT id, family_id
FROM refresh_tokens
WHERE id = $1 AND business_id = $2 AND user_id = $3;

-- name: RevokeRefreshToken :execrows
UPDATE refresh_tokens
SET revoked_at = now()
WHERE id = $1 AND business_id = $2 AND user_id = $3 AND revoked_at IS NULL;

-- name: GetActiveRefreshSession :one
SELECT rt.id, rt.business_id, rt.user_id, rt.expires_at, rt.revoked_at
FROM refresh_tokens rt
JOIN session_families sf ON sf.id = rt.family_id AND sf.business_id = rt.business_id AND sf.user_id = rt.user_id
WHERE rt.id = $1
  AND rt.business_id = $2
  AND rt.user_id = $3
  AND rt.revoked_at IS NULL
  AND rt.expires_at > now()
  AND sf.revoked_at IS NULL;

-- name: GetLogoutSession :one
SELECT rt.id, rt.business_id, rt.user_id, rt.family_id
FROM refresh_tokens rt
JOIN session_families sf ON sf.id = rt.family_id AND sf.business_id = rt.business_id AND sf.user_id = rt.user_id
WHERE rt.id = $1 AND rt.business_id = $2 AND rt.user_id = $3
  AND rt.expires_at > now() AND sf.revoked_at IS NULL;
