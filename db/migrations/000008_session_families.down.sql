DROP INDEX session_families_user_active_idx;
DROP INDEX refresh_tokens_family_idx;
ALTER TABLE refresh_tokens DROP CONSTRAINT refresh_tokens_family_fk;
ALTER TABLE refresh_tokens DROP COLUMN family_id;
COMMENT ON COLUMN refresh_tokens.revoked_at IS 'Timestamp of revocation; NULL means the session has not been revoked.';
DROP TABLE session_families;
