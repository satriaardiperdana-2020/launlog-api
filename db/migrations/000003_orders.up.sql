CREATE TABLE orders (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL,
    outlet_id BIGINT NOT NULL,
    customer_id BIGINT NOT NULL,
    invoice_number TEXT NOT NULL CHECK (btrim(invoice_number) <> ''),
    status TEXT NOT NULL DEFAULT 'RECEIVED'
        CHECK (status IN ('RECEIVED', 'PROCESSING', 'READY_FOR_PICKUP', 'COMPLETED', 'CANCELLED')),
    payment_status TEXT NOT NULL DEFAULT 'UNPAID'
        CHECK (payment_status IN ('UNPAID', 'PARTIALLY_PAID', 'PAID', 'REFUNDED')),
    total_amount BIGINT NOT NULL CHECK (total_amount >= 0),
    notes TEXT,
    received_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    due_at TIMESTAMPTZ,
    completed_at TIMESTAMPTZ,
    cancelled_at TIMESTAMPTZ,
    cancelled_by BIGINT,
    cancellation_reason TEXT,
    deleted_at TIMESTAMPTZ,
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (business_id, id),
    UNIQUE (business_id, outlet_id, id),
    UNIQUE (business_id, outlet_id, invoice_number),
    FOREIGN KEY (business_id, outlet_id)
        REFERENCES outlets (business_id, id),
    FOREIGN KEY (business_id, customer_id)
        REFERENCES customers (business_id, id),
    FOREIGN KEY (business_id, created_by)
        REFERENCES users (business_id, id),
    FOREIGN KEY (business_id, cancelled_by)
        REFERENCES users (business_id, id),
    CHECK (due_at IS NULL OR due_at >= received_at),
    CHECK (
        (status = 'COMPLETED' AND completed_at IS NOT NULL)
        OR (status <> 'COMPLETED' AND completed_at IS NULL)
    ),
    CHECK (
        (
            status = 'CANCELLED'
            AND cancelled_at IS NOT NULL
            AND cancelled_by IS NOT NULL
            AND NULLIF(btrim(cancellation_reason), '') IS NOT NULL
            AND deleted_at IS NOT NULL
        )
        OR (
            status <> 'CANCELLED'
            AND cancelled_at IS NULL
            AND cancelled_by IS NULL
            AND cancellation_reason IS NULL
            AND deleted_at IS NULL
        )
    )
);

CREATE INDEX orders_outlet_created_idx
    ON orders (business_id, outlet_id, created_at DESC);

CREATE INDEX orders_customer_created_idx
    ON orders (business_id, customer_id, created_at DESC);

CREATE INDEX orders_outlet_status_idx
    ON orders (business_id, outlet_id, status, created_at DESC);

CREATE INDEX orders_outlet_payment_status_idx
    ON orders (business_id, outlet_id, payment_status, created_at DESC);

CREATE INDEX orders_outlet_due_idx
    ON orders (business_id, outlet_id, due_at)
    WHERE status NOT IN ('COMPLETED', 'CANCELLED');

CREATE INDEX orders_cancelled_idx
    ON orders (business_id, outlet_id, cancelled_at DESC)
    WHERE status = 'CANCELLED';

CREATE TABLE order_items (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL,
    outlet_id BIGINT NOT NULL,
    order_id BIGINT NOT NULL,
    service_id BIGINT NOT NULL,
    perfume_id BIGINT,
    service_name_snapshot TEXT NOT NULL CHECK (btrim(service_name_snapshot) <> ''),
    service_unit_snapshot TEXT NOT NULL
        CHECK (service_unit_snapshot IN ('KILOGRAM', 'PIECE', 'METER', 'SQUARE_METER')),
    unit_price_amount_snapshot BIGINT NOT NULL CHECK (unit_price_amount_snapshot >= 0),
    perfume_name_snapshot TEXT,
    quantity NUMERIC(12,3) NOT NULL CHECK (quantity > 0),
    line_total_amount BIGINT NOT NULL CHECK (line_total_amount >= 0),
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (business_id, outlet_id, order_id, id),
    FOREIGN KEY (business_id, outlet_id, order_id)
        REFERENCES orders (business_id, outlet_id, id),
    FOREIGN KEY (business_id, service_id)
        REFERENCES services (business_id, id),
    FOREIGN KEY (business_id, perfume_id)
        REFERENCES perfumes (business_id, id),
    CHECK (service_unit_snapshot <> 'PIECE' OR quantity = trunc(quantity)),
    CHECK (line_total_amount = round(quantity * unit_price_amount_snapshot)::BIGINT),
    CHECK (
        (perfume_id IS NULL AND perfume_name_snapshot IS NULL)
        OR (perfume_id IS NOT NULL AND NULLIF(btrim(perfume_name_snapshot), '') IS NOT NULL)
    )
);

CREATE INDEX order_items_order_idx
    ON order_items (business_id, outlet_id, order_id);

CREATE INDEX order_items_service_idx
    ON order_items (business_id, service_id);

CREATE TABLE order_status_history (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL,
    outlet_id BIGINT NOT NULL,
    order_id BIGINT NOT NULL,
    from_status TEXT,
    to_status TEXT NOT NULL,
    changed_by BIGINT NOT NULL,
    notes TEXT,
    changed_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (business_id, outlet_id, order_id, id),
    FOREIGN KEY (business_id, outlet_id, order_id)
        REFERENCES orders (business_id, outlet_id, id),
    FOREIGN KEY (business_id, changed_by)
        REFERENCES users (business_id, id),
    CHECK (from_status IS NULL OR from_status IN ('RECEIVED', 'PROCESSING', 'READY_FOR_PICKUP')),
    CHECK (to_status IN ('RECEIVED', 'PROCESSING', 'READY_FOR_PICKUP', 'COMPLETED', 'CANCELLED')),
    CHECK (
        (from_status IS NULL AND to_status = 'RECEIVED')
        OR (from_status = 'RECEIVED' AND to_status IN ('PROCESSING', 'CANCELLED'))
        OR (from_status = 'PROCESSING' AND to_status IN ('READY_FOR_PICKUP', 'CANCELLED'))
        OR (from_status = 'READY_FOR_PICKUP' AND to_status IN ('COMPLETED', 'CANCELLED'))
    )
);

CREATE INDEX order_status_history_order_idx
    ON order_status_history (business_id, outlet_id, order_id, changed_at);

CREATE INDEX order_status_history_actor_idx
    ON order_status_history (business_id, changed_by, changed_at DESC);

CREATE TABLE invoice_counters (
    business_id BIGINT NOT NULL,
    outlet_id BIGINT NOT NULL,
    counter_date DATE NOT NULL,
    next_number BIGINT NOT NULL DEFAULT 1 CHECK (next_number > 0),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, outlet_id, counter_date),
    FOREIGN KEY (business_id, outlet_id)
        REFERENCES outlets (business_id, id)
);
