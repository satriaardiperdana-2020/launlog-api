DROP TRIGGER orders_receipt_customer_snapshot_before_insert ON orders;
DROP FUNCTION snapshot_order_customer_for_receipt();
DROP INDEX orders_receipt_qr_id_uq;
ALTER TABLE orders
    DROP COLUMN receipt_qr_id,
    DROP COLUMN customer_phone_snapshot,
    DROP COLUMN customer_name_snapshot;
