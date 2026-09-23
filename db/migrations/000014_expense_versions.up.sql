ALTER TABLE expenses
    ADD COLUMN version BIGINT NOT NULL DEFAULT 1 CHECK (version > 0);

COMMENT ON COLUMN expenses.version IS
    'Optimistic concurrency version. Updated atomically for each expense edit to prevent lost updates.';
