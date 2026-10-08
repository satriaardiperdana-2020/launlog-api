CREATE TABLE platform_support_requests (
 id BIGSERIAL PRIMARY KEY,
 business_id BIGINT NOT NULL REFERENCES businesses(id),
 requested_by_user_id BIGINT NOT NULL,
 reason TEXT NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
 access_scope TEXT NOT NULL CHECK (access_scope IN ('READ_ONLY','READ_WRITE')),
 created_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 expires_at TIMESTAMPTZ NOT NULL,
 revoked_at TIMESTAMPTZ,
 revocation_reason TEXT,
 UNIQUE (business_id,id),
 FOREIGN KEY (business_id,requested_by_user_id) REFERENCES users(business_id,id),
 CHECK (expires_at > created_at AND expires_at <= created_at + interval '1 hour')
);
CREATE TABLE platform_support_sessions (
 id BIGSERIAL PRIMARY KEY,
 business_id BIGINT NOT NULL REFERENCES businesses(id),
 support_request_id BIGINT NOT NULL UNIQUE,
 platform_admin_id BIGINT NOT NULL REFERENCES platform_admins(id),
 platform_family_id BIGINT NOT NULL,
 reason TEXT NOT NULL CHECK (length(btrim(reason)) BETWEEN 1 AND 500),
 access_scope TEXT NOT NULL CHECK (access_scope IN ('READ_ONLY','READ_WRITE')),
 started_at TIMESTAMPTZ NOT NULL DEFAULT clock_timestamp(),
 expires_at TIMESTAMPTZ NOT NULL,
 ended_at TIMESTAMPTZ,
 end_reason TEXT,
 UNIQUE (business_id,id),
 FOREIGN KEY (business_id,support_request_id) REFERENCES platform_support_requests(business_id,id),
 FOREIGN KEY (platform_admin_id,platform_family_id) REFERENCES platform_session_families(platform_admin_id,id),
 CHECK (expires_at > started_at AND expires_at <= started_at + interval '1 hour')
);
CREATE INDEX platform_support_requests_business_idx ON platform_support_requests(business_id,id);
CREATE INDEX platform_support_sessions_business_idx ON platform_support_sessions(business_id,id);
ALTER TABLE platform_audit_logs ADD COLUMN business_id BIGINT REFERENCES businesses(id),
 ADD COLUMN support_session_id BIGINT,
 ADD COLUMN reason TEXT,
 ADD CONSTRAINT platform_audit_business_id_uq UNIQUE (business_id,id),
 ADD CONSTRAINT platform_audit_support_fk FOREIGN KEY (business_id,support_session_id) REFERENCES platform_support_sessions(business_id,id),
 ADD CONSTRAINT platform_audit_support_context CHECK (support_session_id IS NULL OR (business_id IS NOT NULL AND reason IS NOT NULL AND btrim(reason) <> ''));
ALTER TABLE audit_logs ADD COLUMN actor_platform_admin_id BIGINT REFERENCES platform_admins(id),
 ADD COLUMN support_session_id BIGINT,
 ADD COLUMN platform_audit_id BIGINT,
 ADD CONSTRAINT audit_actor_separation CHECK (actor_user_id IS NULL OR actor_platform_admin_id IS NULL),
 ADD CONSTRAINT audit_support_fk FOREIGN KEY (business_id,support_session_id) REFERENCES platform_support_sessions(business_id,id),
 ADD CONSTRAINT audit_platform_correlation_fk FOREIGN KEY (business_id,platform_audit_id) REFERENCES platform_audit_logs(business_id,id),
 ADD CONSTRAINT audit_support_actor CHECK (support_session_id IS NULL OR ((actor_platform_admin_id IS NOT NULL OR actor_user_id IS NOT NULL) AND platform_audit_id IS NOT NULL));
ALTER TABLE perfumes ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0);
CREATE TRIGGER platform_audit_logs_no_truncate BEFORE TRUNCATE ON platform_audit_logs FOR EACH STATEMENT EXECUTE FUNCTION prevent_audit_log_mutation();
CREATE TRIGGER audit_logs_no_truncate BEFORE TRUNCATE ON audit_logs FOR EACH STATEMENT EXECUTE FUNCTION prevent_audit_log_mutation();
