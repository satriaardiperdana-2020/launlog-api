-- name: GetPlatformAdminForLogin :one
SELECT id, email, password_hash, is_active FROM platform_admins WHERE email = lower(btrim($1));

-- name: GetActivePlatformAdmin :one
SELECT id, email FROM platform_admins WHERE id = $1 AND is_active;

-- name: CreatePlatformSessionFamily :one
INSERT INTO platform_session_families (platform_admin_id) VALUES ($1) RETURNING id;

-- name: CreatePlatformRefreshToken :one
INSERT INTO platform_refresh_tokens (platform_admin_id, family_id, token_hash, expires_at)
VALUES ($1, $2, $3, $4) RETURNING id, expires_at;

-- name: FindPlatformRefreshFamily :one
SELECT family_id, platform_admin_id FROM platform_refresh_tokens WHERE token_hash = $1;

-- name: LockPlatformSessionFamily :one
SELECT id, platform_admin_id, revoked_at FROM platform_session_families
WHERE id = $1 AND platform_admin_id = $2 FOR UPDATE;

-- name: GetPlatformRefreshForUpdate :one
SELECT id, platform_admin_id, family_id, expires_at, revoked_at FROM platform_refresh_tokens
WHERE token_hash = $1 FOR UPDATE;

-- name: GetPlatformRefreshByID :one
SELECT id, family_id FROM platform_refresh_tokens WHERE id = $1 AND platform_admin_id = $2;

-- name: GetActivePlatformSession :one
SELECT t.id FROM platform_refresh_tokens t
JOIN platform_session_families f ON f.id = t.family_id AND f.platform_admin_id = t.platform_admin_id
WHERE t.id = $1 AND t.platform_admin_id = $2 AND t.revoked_at IS NULL
  AND t.expires_at > now() AND f.revoked_at IS NULL;

-- name: GetPlatformLogoutSession :one
SELECT t.id, t.family_id FROM platform_refresh_tokens t
JOIN platform_session_families f ON f.id = t.family_id AND f.platform_admin_id = t.platform_admin_id
WHERE t.id = $1 AND t.platform_admin_id = $2 AND t.expires_at > now() AND f.revoked_at IS NULL;

-- name: RevokePlatformRefreshToken :execrows
UPDATE platform_refresh_tokens SET revoked_at = now()
WHERE id = $1 AND platform_admin_id = $2 AND revoked_at IS NULL;

-- name: RevokePlatformSessionFamily :execrows
UPDATE platform_session_families SET revoked_at = now()
WHERE id = $1 AND platform_admin_id = $2 AND revoked_at IS NULL;

-- name: InsertPlatformAuditLog :exec
INSERT INTO platform_audit_logs (actor_platform_admin_id, action, target_type, target_id, outcome, request_id, metadata)
VALUES ($1, $2, $3, $4, $5, $6, '{}'::jsonb);
