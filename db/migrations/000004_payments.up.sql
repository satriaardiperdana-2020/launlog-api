CREATE TABLE payments (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL,
    outlet_id BIGINT NOT NULL,
    order_id BIGINT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    method TEXT NOT NULL CHECK (method IN ('CASH', 'BCA_TRANSFER', 'QRIS')),
    status TEXT NOT NULL DEFAULT 'CONFIRMED'
        CHECK (status IN ('PENDING', 'CONFIRMED', 'VOIDED')),
    external_reference TEXT,
    notes TEXT,
    confirmed_at TIMESTAMPTZ,
    voided_at TIMESTAMPTZ,
    voided_by BIGINT,
    void_reason TEXT,
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (business_id, outlet_id, order_id, id),
    FOREIGN KEY (business_id, outlet_id, order_id)
        REFERENCES orders (business_id, outlet_id, id),
    FOREIGN KEY (business_id, created_by)
        REFERENCES users (business_id, id),
    FOREIGN KEY (business_id, voided_by)
        REFERENCES users (business_id, id),
    CHECK (
        (status = 'PENDING' AND confirmed_at IS NULL AND voided_at IS NULL AND voided_by IS NULL AND void_reason IS NULL)
        OR (status = 'CONFIRMED' AND confirmed_at IS NOT NULL AND voided_at IS NULL AND voided_by IS NULL AND void_reason IS NULL)
        OR (
            status = 'VOIDED'
            AND voided_at IS NOT NULL
            AND voided_by IS NOT NULL
            AND NULLIF(btrim(void_reason), '') IS NOT NULL
        )
    )
);

CREATE INDEX payments_order_idx
    ON payments (business_id, outlet_id, order_id, created_at);

CREATE INDEX payments_confirmed_date_idx
    ON payments (business_id, outlet_id, confirmed_at DESC)
    WHERE status = 'CONFIRMED';

CREATE INDEX payments_method_date_idx
    ON payments (business_id, outlet_id, method, confirmed_at DESC)
    WHERE status = 'CONFIRMED';

CREATE TABLE payment_refunds (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL,
    outlet_id BIGINT NOT NULL,
    order_id BIGINT NOT NULL,
    payment_id BIGINT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    reason TEXT NOT NULL CHECK (btrim(reason) <> ''),
    refunded_by BIGINT NOT NULL,
    refunded_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (business_id, outlet_id, order_id, payment_id, id),
    FOREIGN KEY (business_id, outlet_id, order_id, payment_id)
        REFERENCES payments (business_id, outlet_id, order_id, id),
    FOREIGN KEY (business_id, refunded_by)
        REFERENCES users (business_id, id)
);

CREATE INDEX payment_refunds_payment_idx
    ON payment_refunds (business_id, outlet_id, order_id, payment_id, refunded_at);

CREATE INDEX payment_refunds_date_idx
    ON payment_refunds (business_id, outlet_id, refunded_at DESC);
