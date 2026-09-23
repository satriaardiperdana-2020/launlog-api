ALTER TABLE payments
    ADD COLUMN idempotency_key TEXT,
    ADD COLUMN request_hash BYTEA;

ALTER TABLE payments
    ADD CONSTRAINT payments_idempotency_pair_ck
    CHECK (
        (idempotency_key IS NULL AND request_hash IS NULL)
        OR (
            idempotency_key IS NOT NULL
            AND btrim(idempotency_key) <> ''
            AND length(idempotency_key) <= 128
            AND request_hash IS NOT NULL
            AND octet_length(request_hash) = 32
        )
    );

CREATE UNIQUE INDEX payments_idempotency_uq
    ON payments (business_id, outlet_id, order_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL;

COMMENT ON COLUMN payments.idempotency_key IS 'Client retry key scoped to business, outlet, and order; NULL for legacy rows.';
COMMENT ON COLUMN payments.request_hash IS 'SHA-256 hash of the normalized payment request used to detect idempotency-key payload conflicts.';
COMMENT ON COLUMN payments.amount IS 'Whole rupiah credited to the order; for cash, tender above the outstanding balance is excluded and returned as change.';
COMMENT ON COLUMN payments.method IS 'Payment method constrained to CASH, BCA_TRANSFER, or QRIS; BCA_TRANSFER is displayed as BCA Transfer in UI.';
