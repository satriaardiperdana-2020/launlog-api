DROP TRIGGER audit_logs_no_truncate ON audit_logs;
DROP TRIGGER platform_audit_logs_no_truncate ON platform_audit_logs;
ALTER TABLE perfumes DROP COLUMN version;
ALTER TABLE audit_logs DROP CONSTRAINT audit_support_actor, DROP CONSTRAINT audit_platform_correlation_fk, DROP CONSTRAINT audit_support_fk, DROP CONSTRAINT audit_actor_separation, DROP COLUMN actor_platform_admin_id, DROP COLUMN support_session_id, DROP COLUMN platform_audit_id;
ALTER TABLE platform_audit_logs DROP CONSTRAINT platform_audit_support_context, DROP CONSTRAINT platform_audit_support_fk, DROP CONSTRAINT platform_audit_business_id_uq, DROP COLUMN reason, DROP COLUMN support_session_id, DROP COLUMN business_id;
DROP TABLE platform_support_sessions;
DROP TABLE platform_support_requests;
