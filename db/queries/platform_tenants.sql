-- name: PlatformListBusinesses :many
SELECT id,name,timezone,phone,address,is_active,created_at,updated_at FROM businesses WHERE (sqlc.narg(active)::boolean IS NULL OR is_active=sqlc.narg(active)) AND name ILIKE '%' || sqlc.arg(search)::text || '%' ORDER BY id LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
-- name: PlatformCountBusinesses :one
SELECT count(*) FROM businesses WHERE (sqlc.narg(active)::boolean IS NULL OR is_active=sqlc.narg(active)) AND name ILIKE '%' || sqlc.arg(search)::text || '%';
-- name: PlatformGetBusiness :one
SELECT id,name,timezone,phone,address,is_active,created_at,updated_at FROM businesses WHERE id=$1;
-- name: PlatformCreateBusiness :one
INSERT INTO businesses(name,phone,address) VALUES ($1,$2,$3) RETURNING id,name,timezone,phone,address,is_active,created_at,updated_at;
-- name: PlatformCreateFirstAdmin :one
INSERT INTO users(business_id,email,full_name,password_hash,role) VALUES ($1,$2,$3,$4,'ADMIN') RETURNING id,business_id,email,full_name,role,is_active;
-- name: PlatformLockBusiness :one
SELECT id,is_active FROM businesses WHERE id=$1 FOR UPDATE;
-- name: PlatformSetBusinessActive :exec
UPDATE businesses SET is_active=$2,updated_at=clock_timestamp() WHERE id=$1;
-- name: PlatformRevokeTenantFamilies :exec
UPDATE session_families SET revoked_at=clock_timestamp() WHERE business_id=$1 AND revoked_at IS NULL;
-- name: PlatformRevokeBusinessSupport :exec
UPDATE platform_support_requests SET revoked_at=clock_timestamp(),revocation_reason='Business deactivated' WHERE business_id=$1 AND revoked_at IS NULL;
-- name: SupportCreateRequest :one
INSERT INTO platform_support_requests(business_id,requested_by_user_id,reason,access_scope,expires_at) VALUES ($1,$2,$3,$4,$5) RETURNING *;
-- name: SupportLockOwner :one
SELECT u.id,u.password_hash FROM users u JOIN businesses b ON b.id=u.business_id WHERE u.business_id=$1 AND u.id=$2 AND u.role='ADMIN' AND u.is_active AND b.is_active FOR SHARE OF b,u;
-- name: SupportLockRequest :one
SELECT r.* FROM platform_support_requests r JOIN businesses b ON b.id=r.business_id JOIN users u ON u.business_id=r.business_id AND u.id=r.requested_by_user_id WHERE r.business_id=$1 AND r.id=$2 AND r.revoked_at IS NULL AND r.expires_at>clock_timestamp() AND b.is_active AND u.is_active AND u.role='ADMIN' FOR UPDATE OF r FOR SHARE OF b,u;
-- name: SupportGetPlatformFamily :one
SELECT t.family_id FROM platform_refresh_tokens t JOIN platform_session_families f ON f.id=t.family_id AND f.platform_admin_id=t.platform_admin_id WHERE t.id=$1 AND t.platform_admin_id=$2 AND t.revoked_at IS NULL AND t.expires_at>clock_timestamp() AND f.revoked_at IS NULL FOR SHARE OF f,t;
-- name: SupportCreateSession :one
INSERT INTO platform_support_sessions(business_id,support_request_id,platform_admin_id,platform_family_id,reason,access_scope,expires_at) VALUES ($1,$2,$3,$4,$5,$6,$7) RETURNING *;
-- name: SupportLockSession :one
SELECT s.* FROM platform_support_sessions s JOIN platform_support_requests r ON r.business_id=s.business_id AND r.id=s.support_request_id JOIN businesses b ON b.id=s.business_id JOIN users u ON u.business_id=r.business_id AND u.id=r.requested_by_user_id WHERE s.business_id=$1 AND s.id=$2 AND s.platform_admin_id=$3 AND s.platform_family_id=$4 AND s.ended_at IS NULL AND s.expires_at>clock_timestamp() AND r.revoked_at IS NULL AND r.expires_at>clock_timestamp() AND b.is_active AND u.is_active AND u.role='ADMIN' FOR SHARE OF b,r,s,u;
-- name: SupportSessionStillValid :one
SELECT EXISTS(SELECT 1 FROM platform_support_sessions s JOIN platform_support_requests r ON r.id=s.support_request_id WHERE s.id=$1 AND s.ended_at IS NULL AND s.expires_at>clock_timestamp() AND r.revoked_at IS NULL AND r.expires_at>clock_timestamp());
-- name: SupportEndSession :one
UPDATE platform_support_sessions SET ended_at=COALESCE(ended_at,clock_timestamp()),end_reason=COALESCE(end_reason,$3) WHERE business_id=$1 AND id=$2 RETURNING *;
-- name: SupportRevokeRequest :one
UPDATE platform_support_requests SET revoked_at=COALESCE(revoked_at,clock_timestamp()),revocation_reason=COALESCE(revocation_reason,$3) WHERE business_id=$1 AND id=$2 RETURNING *;
-- name: SupportListRequests :many
SELECT * FROM platform_support_requests WHERE business_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: SupportListSessions :many
SELECT * FROM platform_support_sessions WHERE business_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: SupportCountRequests :one
SELECT count(*) FROM platform_support_requests WHERE business_id=$1;
-- name: SupportCountSessions :one
SELECT count(*) FROM platform_support_sessions WHERE business_id=$1;
-- name: PlatformWriteTenantAudit :one
INSERT INTO platform_audit_logs(actor_platform_admin_id,business_id,support_session_id,reason,action,target_type,target_id,outcome,request_id,metadata) VALUES ($1,$2,$3,$4,$5,$6,$7,'SUCCESS',$8,$9) RETURNING id;
-- name: BusinessWritePlatformAudit :exec
INSERT INTO audit_logs(business_id,actor_user_id,actor_platform_admin_id,support_session_id,platform_audit_id,action,entity_type,entity_id,new_values) VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9);
-- name: SupportGetPerfume :one
SELECT id,business_id,name,description,is_active,version FROM perfumes WHERE business_id=$1 AND id=$2 AND deleted_at IS NULL;
-- name: SupportUpdatePerfumeDescription :one
UPDATE perfumes SET description=$3,version=version+1,updated_at=clock_timestamp() WHERE business_id=$1 AND id=$2 AND version=$4 AND deleted_at IS NULL RETURNING id,business_id,name,description,is_active,version;
-- name: PlatformAuditView :many
SELECT id,business_id,actor_platform_admin_id,support_session_id,reason,action,target_type,target_id,outcome,request_id,metadata,occurred_at FROM platform_audit_logs WHERE (sqlc.narg(business_id)::bigint IS NULL OR business_id=sqlc.narg(business_id)) ORDER BY id DESC LIMIT sqlc.arg(page_limit) OFFSET sqlc.arg(page_offset);
-- name: PlatformAuditCount :one
SELECT count(*) FROM platform_audit_logs WHERE (sqlc.narg(business_id)::bigint IS NULL OR business_id=sqlc.narg(business_id));
-- name: SupportDiagnostics :one
SELECT (SELECT count(*) FROM outlets o WHERE o.business_id=b.id)::bigint AS outlets,(SELECT count(*) FROM users u WHERE u.business_id=b.id)::bigint AS users,(SELECT count(*) FROM perfumes p WHERE p.business_id=b.id AND p.deleted_at IS NULL)::bigint AS perfumes FROM businesses b WHERE b.id=$1;
-- name: SupportGetRequest :one
SELECT * FROM platform_support_requests WHERE business_id=$1 AND id=$2;
-- name: SupportListPerfumes :many
SELECT id,business_id,name,description,is_active,version FROM perfumes WHERE business_id=$1 AND deleted_at IS NULL ORDER BY id LIMIT $2 OFFSET $3;
-- name: SupportCountPerfumes :one
SELECT count(*) FROM perfumes WHERE business_id=$1 AND deleted_at IS NULL;
-- name: PlatformBusinessCounts :one
SELECT (SELECT count(*) FROM outlets o WHERE o.business_id=b.id)::bigint AS outlets,(SELECT count(*) FROM users u WHERE u.business_id=b.id AND u.role='ADMIN' AND u.is_active)::bigint AS active_admins FROM businesses b WHERE b.id=$1;
-- name: BusinessAuditView :many
SELECT id,business_id,outlet_id,actor_user_id,actor_platform_admin_id,support_session_id,platform_audit_id,action,entity_type,entity_id,occurred_at FROM audit_logs WHERE business_id=$1 ORDER BY id DESC LIMIT $2 OFFSET $3;
-- name: BusinessAuditCount :one
SELECT count(*) FROM audit_logs WHERE business_id=$1;
