CREATE INDEX orders_outlet_received_idx
    ON orders (business_id, outlet_id, received_at DESC);

CREATE INDEX payments_confirmed_order_idx
    ON payments (business_id, outlet_id, order_id)
    WHERE status = 'CONFIRMED';

COMMENT ON INDEX orders_outlet_received_idx IS
    'Supports outlet-scoped order cohort reports filtered by received_at.';
COMMENT ON INDEX payments_confirmed_order_idx IS
    'Supports confirmed collection aggregation by order without scanning pending or voided receipts.';
