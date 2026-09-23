CREATE TABLE session_families (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at TIMESTAMPTZ,
    UNIQUE (business_id, user_id, id),
    FOREIGN KEY (business_id, user_id) REFERENCES users (business_id, id),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

COMMENT ON TABLE session_families IS 'Stable session lineage; refresh rotation and logout serialize on this row.';
COMMENT ON COLUMN session_families.revoked_at IS 'Revokes every access and refresh token in this family, including consumed descendants.';

-- Preserve every existing token hash and session. Each legacy token starts its
-- own family because no trustworthy rotation ancestry was previously recorded.
INSERT INTO session_families (id, business_id, user_id, created_at, revoked_at)
SELECT id, business_id, user_id, created_at, revoked_at FROM refresh_tokens;

SELECT setval(pg_get_serial_sequence('session_families', 'id'),
    GREATEST(COALESCE((SELECT max(id) FROM session_families), 0), 1),
    EXISTS (SELECT 1 FROM session_families WHERE id > 0));

ALTER TABLE refresh_tokens ADD COLUMN family_id BIGINT;
UPDATE refresh_tokens SET family_id = id;
ALTER TABLE refresh_tokens ALTER COLUMN family_id SET NOT NULL;
ALTER TABLE refresh_tokens ADD CONSTRAINT refresh_tokens_family_fk
    FOREIGN KEY (business_id, user_id, family_id)
    REFERENCES session_families (business_id, user_id, id);

CREATE INDEX refresh_tokens_family_idx ON refresh_tokens (family_id, id);
CREATE INDEX session_families_user_active_idx
    ON session_families (business_id, user_id, id) WHERE revoked_at IS NULL;

COMMENT ON COLUMN refresh_tokens.family_id IS 'Stable family across refresh rotations; historical token hashes remain stored for replay detection.';
COMMENT ON COLUMN refresh_tokens.revoked_at IS 'Token consumed by rotation or revoked by logout; retained for replay detection.';
