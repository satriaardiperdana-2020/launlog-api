DROP INDEX orders_idempotency_key_uq;
ALTER TABLE orders
    DROP CONSTRAINT orders_idempotency_pair_check,
    DROP COLUMN request_hash,
    DROP COLUMN idempotency_key;
