BEGIN;

-- Convert the unit code only; preserve historical names, prices and quantities.
ALTER TABLE services DROP CONSTRAINT services_unit_check;
ALTER TABLE order_items DROP CONSTRAINT order_items_service_unit_snapshot_check;
ALTER TABLE order_items DROP CONSTRAINT order_items_check;

UPDATE services SET unit = CASE unit
    WHEN 'm2' THEN 'SQUARE_METER'
    WHEN 'kg' THEN 'KILOGRAM'
    WHEN 'pcs' THEN 'PIECE'
    WHEN 'm' THEN 'METER'
    ELSE unit END;
UPDATE order_items SET service_unit_snapshot = CASE service_unit_snapshot
    WHEN 'm2' THEN 'SQUARE_METER'
    WHEN 'kg' THEN 'KILOGRAM'
    WHEN 'pcs' THEN 'PIECE'
    WHEN 'm' THEN 'METER'
    ELSE service_unit_snapshot END;

ALTER TABLE services ADD CONSTRAINT services_unit_check
    CHECK (unit IN ('KILOGRAM', 'PIECE', 'METER', 'SQUARE_METER'));
ALTER TABLE order_items ADD CONSTRAINT order_items_service_unit_snapshot_check
    CHECK (service_unit_snapshot IN ('KILOGRAM', 'PIECE', 'METER', 'SQUARE_METER'));
ALTER TABLE order_items ADD CONSTRAINT order_items_check
    CHECK (service_unit_snapshot <> 'PIECE' OR quantity = trunc(quantity));

COMMENT ON COLUMN services.unit IS
    'Measurement type constrained to KILOGRAM, PIECE, METER, or SQUARE_METER.';

COMMIT;
