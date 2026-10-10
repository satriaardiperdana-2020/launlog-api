DO $$
BEGIN
    IF EXISTS (SELECT 1 FROM outlets WHERE legacy_code IS NULL) THEN
        RAISE EXCEPTION 'cannot restore legacy outlet codes: outlets created after migration have no legacy_code';
    END IF;
END $$;

ALTER TABLE orders DROP COLUMN outlet_code_snapshot;
DROP TABLE business_outlet_counters;
DROP INDEX outlets_business_id_legacy_code_uq;
ALTER TABLE outlets DROP CONSTRAINT outlets_business_id_code_key;
UPDATE outlets SET code = legacy_code;
ALTER TABLE outlets ADD CONSTRAINT outlets_business_id_code_key UNIQUE (business_id, code);
ALTER TABLE outlets DROP COLUMN legacy_code;
