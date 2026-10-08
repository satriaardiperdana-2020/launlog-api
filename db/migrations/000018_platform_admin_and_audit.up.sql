CREATE TABLE platform_admins (
    id BIGINT PRIMARY KEY DEFAULT 1 CHECK (id = 1),
    email TEXT NOT NULL CHECK (email = lower(btrim(email)) AND email <> ''),
    password_hash TEXT NOT NULL CHECK (btrim(password_hash) <> ''),
    is_active BOOLEAN NOT NULL DEFAULT TRUE CHECK (is_active),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE UNIQUE INDEX platform_admins_email_uq ON platform_admins (lower(email));

CREATE TABLE platform_session_families (
    id BIGSERIAL PRIMARY KEY,
    platform_admin_id BIGINT NOT NULL REFERENCES platform_admins (id),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    UNIQUE (platform_admin_id, id),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE TABLE platform_refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    platform_admin_id BIGINT NOT NULL,
    family_id BIGINT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE CHECK (btrim(token_hash) <> ''),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (platform_admin_id, family_id)
        REFERENCES platform_session_families (platform_admin_id, id),
    CHECK (expires_at > created_at),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);
CREATE INDEX platform_refresh_tokens_family_idx ON platform_refresh_tokens (family_id, id);
CREATE INDEX platform_refresh_tokens_active_idx ON platform_refresh_tokens (platform_admin_id, expires_at) WHERE revoked_at IS NULL;

CREATE TABLE platform_audit_logs (
    id BIGSERIAL PRIMARY KEY,
    actor_platform_admin_id BIGINT REFERENCES platform_admins (id),
    action TEXT NOT NULL CHECK (btrim(action) <> ''),
    target_type TEXT NOT NULL CHECK (btrim(target_type) <> ''),
    target_id BIGINT,
    outcome TEXT NOT NULL CHECK (outcome IN ('SUCCESS', 'DENIED')),
    request_id TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb CHECK (jsonb_typeof(metadata) = 'object'),
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now()
);
CREATE INDEX platform_audit_logs_actor_date_idx ON platform_audit_logs (actor_platform_admin_id, occurred_at DESC);
CREATE INDEX platform_audit_logs_target_date_idx ON platform_audit_logs (target_type, target_id, occurred_at DESC);
CREATE TRIGGER platform_audit_logs_immutable
    BEFORE UPDATE OR DELETE ON platform_audit_logs
    FOR EACH ROW EXECUTE FUNCTION prevent_audit_log_mutation();

COMMENT ON TABLE platform_admins IS 'Singleton platform operator, never a tenant user. Bootstrap creates id 1 exactly once.';
COMMENT ON TABLE platform_audit_logs IS 'Append-only platform security history without credentials or tenant payloads.';
