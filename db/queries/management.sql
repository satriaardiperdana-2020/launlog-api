-- name: ListOutlets :many
SELECT id, business_id, code, name, phone, address, is_active, created_at, updated_at
FROM outlets WHERE business_id = $1 ORDER BY id LIMIT $2 OFFSET $3;

-- name: CountOutlets :one
SELECT count(*) FROM outlets WHERE business_id = $1;

-- name: GetOutlet :one
SELECT id, business_id, code, name, phone, address, is_active, created_at, updated_at
FROM outlets WHERE business_id = $1 AND id = $2;

-- name: GetOutletForUpdate :one
SELECT id, business_id, code, name, phone, address, is_active
FROM outlets WHERE business_id = $1 AND id = $2 FOR UPDATE;

-- name: CreateOutlet :one
INSERT INTO outlets (business_id, code, name, phone, address)
VALUES ($1, $2, $3, $4, $5)
RETURNING id, business_id, code, name, phone, address, is_active, created_at, updated_at;

-- name: UpdateOutlet :one
UPDATE outlets SET code=$3, name=$4, phone=$5, address=$6, is_active=$7, updated_at=now()
WHERE business_id=$1 AND id=$2
RETURNING id, business_id, code, name, phone, address, is_active, created_at, updated_at;

-- name: CountActiveAssignedStaffWithoutOtherOutlet :one
SELECT count(DISTINCT u.id)
FROM users u
JOIN user_outlets uo ON uo.business_id=u.business_id AND uo.user_id=u.id
WHERE u.business_id=$1 AND u.is_active AND u.role='LAUNDRY_STAFF' AND uo.outlet_id=$2
AND NOT EXISTS (
    SELECT 1 FROM user_outlets other_uo
    JOIN outlets other ON other.id=other_uo.outlet_id AND other.business_id=other_uo.business_id AND other.is_active
    WHERE other_uo.business_id=u.business_id AND other_uo.user_id=u.id AND other_uo.outlet_id<>$2
);

-- name: ListStaff :many
SELECT id, business_id, email, full_name, role, is_active, created_at, updated_at
FROM users WHERE business_id=$1 ORDER BY id LIMIT $2 OFFSET $3;

-- name: CountStaff :one
SELECT count(*) FROM users WHERE business_id=$1;

-- name: GetStaffForUpdate :one
SELECT id, business_id, email, full_name, role, is_active
FROM users WHERE business_id=$1 AND id=$2 FOR UPDATE;

-- name: GetStaff :one
SELECT id, business_id, email, full_name, role, is_active
FROM users WHERE business_id=$1 AND id=$2;

-- name: LockActiveBusiness :one
SELECT id FROM businesses WHERE id=$1 AND is_active FOR UPDATE;

-- name: CreateStaff :one
INSERT INTO users (business_id,email,full_name,password_hash,role)
VALUES ($1,$2,$3,$4,'LAUNDRY_STAFF')
RETURNING id,business_id,email,full_name,role,is_active,created_at,updated_at;

-- name: UpdateStaff :one
UPDATE users SET full_name=$3,is_active=$4,updated_at=now()
WHERE business_id=$1 AND id=$2
RETURNING id,business_id,email,full_name,role,is_active,created_at,updated_at;

-- name: CountOtherActiveAdmins :one
SELECT count(*) FROM users WHERE business_id=$1 AND role='ADMIN' AND is_active AND id<>$2;

-- name: ListStaffOutletIDs :many
SELECT uo.outlet_id FROM user_outlets uo
JOIN outlets o ON o.business_id=uo.business_id AND o.id=uo.outlet_id AND o.is_active
WHERE uo.business_id=$1 AND uo.user_id=$2 ORDER BY uo.outlet_id;

-- name: DeleteStaffOutlets :exec
DELETE FROM user_outlets WHERE business_id=$1 AND user_id=$2;

-- name: AddStaffOutlet :exec
INSERT INTO user_outlets (business_id,user_id,outlet_id) VALUES ($1,$2,$3);

-- name: CountActiveOutlets :one
SELECT count(DISTINCT id) FROM outlets WHERE business_id=$1 AND is_active AND id=ANY($2::bigint[]);

-- name: LockActiveOutlets :many
SELECT id FROM outlets WHERE business_id=$1 AND is_active AND id=ANY($2::bigint[]) FOR SHARE;

-- name: ListPermissions :many
SELECT code,description FROM permissions ORDER BY code;

-- name: ListStaffPermissionCodes :many
SELECT permission_code FROM user_permissions WHERE business_id=$1 AND user_id=$2 ORDER BY permission_code;

-- name: DeleteStaffPermissions :exec
DELETE FROM user_permissions WHERE business_id=$1 AND user_id=$2;

-- name: AddStaffPermission :exec
INSERT INTO user_permissions (business_id,user_id,permission_code,granted_by)
VALUES ($1,$2,$3,$4);

-- name: InsertAuditLog :exec
INSERT INTO audit_logs (business_id,actor_user_id,action,entity_type,entity_id,old_values,new_values,ip_address,user_agent)
VALUES ($1,$2,$3,$4,$5,$6,$7,$8,$9);
