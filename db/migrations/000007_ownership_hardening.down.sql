DROP INDEX refresh_tokens_business_user_idx;

COMMENT ON TABLE businesses IS NULL;
COMMENT ON COLUMN businesses.id IS NULL;
COMMENT ON COLUMN businesses.timezone IS NULL;
COMMENT ON COLUMN businesses.is_active IS NULL;

COMMENT ON TABLE outlets IS NULL;
COMMENT ON COLUMN outlets.business_id IS NULL;
COMMENT ON COLUMN outlets.code IS NULL;
COMMENT ON COLUMN outlets.is_active IS NULL;

COMMENT ON TABLE users IS NULL;
COMMENT ON COLUMN users.business_id IS NULL;
COMMENT ON COLUMN users.email IS NULL;
COMMENT ON COLUMN users.password_hash IS NULL;
COMMENT ON COLUMN users.role IS NULL;
COMMENT ON COLUMN users.is_active IS NULL;

COMMENT ON TABLE user_outlets IS NULL;
COMMENT ON COLUMN user_outlets.business_id IS NULL;
COMMENT ON COLUMN user_outlets.user_id IS NULL;
COMMENT ON COLUMN user_outlets.outlet_id IS NULL;

COMMENT ON TABLE permissions IS NULL;
COMMENT ON COLUMN permissions.code IS NULL;

COMMENT ON TABLE user_permissions IS NULL;
COMMENT ON COLUMN user_permissions.business_id IS NULL;
COMMENT ON COLUMN user_permissions.user_id IS NULL;
COMMENT ON COLUMN user_permissions.granted_by IS NULL;

COMMENT ON TABLE refresh_tokens IS NULL;
COMMENT ON COLUMN refresh_tokens.business_id IS NULL;
COMMENT ON COLUMN refresh_tokens.user_id IS NULL;
COMMENT ON COLUMN refresh_tokens.token_hash IS NULL;
COMMENT ON COLUMN refresh_tokens.expires_at IS NULL;
COMMENT ON COLUMN refresh_tokens.revoked_at IS NULL;

COMMENT ON TABLE services IS NULL;
COMMENT ON COLUMN services.business_id IS NULL;
COMMENT ON COLUMN services.unit IS NULL;
