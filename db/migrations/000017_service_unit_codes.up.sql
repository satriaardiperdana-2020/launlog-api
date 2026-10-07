BEGIN;

-- Convert the unit code only; preserve historical names, prices and quantities.
ALTER TABLE services DROP CONSTRAINT services_unit_check;
ALTER TABLE order_items DROP CONSTRAINT order_items_service_unit_snapshot_check;
ALTER TABLE order_items DROP CONSTRAINT order_items_check;

UPDATE services SET unit = CASE unit
    WHEN 'SQUARE_METER' THEN 'm2'
    WHEN 'KILOGRAM' THEN 'kg'
    WHEN 'PIECE' THEN 'pcs'
    WHEN 'METER' THEN 'm'
    ELSE unit END;
UPDATE order_items SET service_unit_snapshot = CASE service_unit_snapshot
    WHEN 'SQUARE_METER' THEN 'm2'
    WHEN 'KILOGRAM' THEN 'kg'
    WHEN 'PIECE' THEN 'pcs'
    WHEN 'METER' THEN 'm'
    ELSE service_unit_snapshot END;

ALTER TABLE services ADD CONSTRAINT services_unit_check
    CHECK (unit IN ('kg', 'pcs', 'm', 'm2'));
ALTER TABLE order_items ADD CONSTRAINT order_items_service_unit_snapshot_check
    CHECK (service_unit_snapshot IN ('kg', 'pcs', 'm', 'm2'));
ALTER TABLE order_items ADD CONSTRAINT order_items_check
    CHECK (service_unit_snapshot <> 'pcs' OR quantity = trunc(quantity));

COMMENT ON COLUMN services.unit IS
    'Measurement type constrained to kg, pcs, m, or m2.';

COMMIT;
