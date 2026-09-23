ALTER TABLE orders
    ADD COLUMN idempotency_key TEXT,
    ADD COLUMN request_hash BYTEA,
    ADD CONSTRAINT orders_idempotency_pair_check
        CHECK ((idempotency_key IS NULL AND request_hash IS NULL)
            OR (idempotency_key IS NOT NULL AND request_hash IS NOT NULL
                AND btrim(idempotency_key) <> '' AND octet_length(request_hash) = 32));

CREATE UNIQUE INDEX orders_idempotency_key_uq
    ON orders (business_id, outlet_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

COMMENT ON COLUMN orders.idempotency_key IS
    'Client-supplied request idempotency key, unique within a business and outlet.';
COMMENT ON COLUMN orders.request_hash IS
    'SHA-256 hash of the canonical decoded order creation payload used to reject key reuse with a different payload.';
