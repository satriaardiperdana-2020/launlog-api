-- This migration is intentionally additive. The original ownership tables may
-- already contain production data, so no business, user, outlet, or session is
-- rewritten or deleted here.

-- The existing partial active-session index excludes revoked sessions. This
-- index covers the user foreign key and administrative/session-history lookups.
CREATE INDEX refresh_tokens_business_user_idx
    ON refresh_tokens (business_id, user_id);

COMMENT ON TABLE businesses IS
    'Root tenant. All business-owned records are scoped to one business.';
COMMENT ON COLUMN businesses.id IS
    'Stable BIGINT tenant identifier.';
COMMENT ON COLUMN businesses.timezone IS
    'Business timezone; the application currently permits only Asia/Jakarta.';
COMMENT ON COLUMN businesses.is_active IS
    'False prevents the tenant from being used without deleting its records.';

COMMENT ON TABLE outlets IS
    'A physical or operational location owned by exactly one business.';
COMMENT ON COLUMN outlets.business_id IS
    'Tenant owner; paired with id by tenant-aware foreign keys.';
COMMENT ON COLUMN outlets.code IS
    'Business-local outlet code, unique within business_id.';
COMMENT ON COLUMN outlets.is_active IS
    'False disables new outlet activity without deleting historical records.';

COMMENT ON TABLE users IS
    'Authenticated account owned by one business; it is not a global operator account.';
COMMENT ON COLUMN users.business_id IS
    'Tenant owner; prevents a user from belonging to multiple businesses.';
COMMENT ON COLUMN users.email IS
    'Case-insensitively unique login identifier; unique globally by design so login needs no business selector.';
COMMENT ON COLUMN users.password_hash IS
    'One-way password hash only; plaintext passwords and reversible secrets are never stored.';
COMMENT ON COLUMN users.role IS
    'Coarse business role, constrained to ADMIN or LAUNDRY_STAFF.';
COMMENT ON COLUMN users.is_active IS
    'False blocks authentication and authorization while preserving history.';

COMMENT ON TABLE user_outlets IS
    'Tenant-safe assignment of a user to an outlet in the same business.';
COMMENT ON COLUMN user_outlets.business_id IS
    'Shared tenant key required by both user and outlet foreign keys.';
COMMENT ON COLUMN user_outlets.user_id IS
    'Assigned user; must belong to business_id.';
COMMENT ON COLUMN user_outlets.outlet_id IS
    'Assigned outlet; must belong to business_id.';

COMMENT ON TABLE permissions IS
    'Global catalog of permission codes; grants remain tenant-scoped in user_permissions.';
COMMENT ON COLUMN permissions.code IS
    'Stable uppercase permission identifier, not a tenant identifier.';

COMMENT ON TABLE user_permissions IS
    'Explicit tenant-scoped permission grant. Authorization still validates actor role and outlet scope.';
COMMENT ON COLUMN user_permissions.business_id IS
    'Shared tenant key for recipient and grantor foreign keys.';
COMMENT ON COLUMN user_permissions.user_id IS
    'Permission recipient; must belong to business_id.';
COMMENT ON COLUMN user_permissions.granted_by IS
    'Granting user; must belong to business_id. Whether the actor may grant is enforced by application authorization.';

COMMENT ON TABLE refresh_tokens IS
    'Revocable refresh sessions. token_hash stores a one-way hash, never a raw refresh token.';
COMMENT ON COLUMN refresh_tokens.business_id IS
    'Tenant key paired with user_id for a tenant-aware session foreign key.';
COMMENT ON COLUMN refresh_tokens.user_id IS
    'Session owner; must belong to business_id.';
COMMENT ON COLUMN refresh_tokens.token_hash IS
    'Unique one-way hash of the opaque refresh token.';
COMMENT ON COLUMN refresh_tokens.expires_at IS
    'Exclusive refresh-session expiry timestamp.';
COMMENT ON COLUMN refresh_tokens.revoked_at IS
    'Timestamp of revocation; NULL means the session has not been revoked.';

COMMENT ON TABLE services IS
    'Business-shared service catalog. A service applies to every outlet in its business; outlet-specific pricing is not modelled.';
COMMENT ON COLUMN services.business_id IS
    'Tenant owner. There is intentionally no outlet_id because services are shared across the business.';
COMMENT ON COLUMN services.unit IS
    'Measurement type constrained to KILOGRAM, PIECE, METER, or SQUARE_METER.';
