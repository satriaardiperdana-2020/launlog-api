DROP INDEX payments_idempotency_uq;
COMMENT ON COLUMN payments.amount IS NULL;
COMMENT ON COLUMN payments.method IS NULL;
ALTER TABLE payments DROP CONSTRAINT payments_idempotency_pair_ck;
ALTER TABLE payments DROP COLUMN request_hash, DROP COLUMN idempotency_key;
