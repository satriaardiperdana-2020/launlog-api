ALTER TABLE outlets ADD COLUMN legacy_code TEXT;

LOCK TABLE outlets IN ACCESS EXCLUSIVE MODE;

UPDATE outlets SET legacy_code = code;

ALTER TABLE outlets DROP CONSTRAINT outlets_business_id_code_key;

WITH ranked AS (
    SELECT id,
           CASE WHEN length(row_number() OVER (PARTITION BY business_id ORDER BY created_at, id)::text) < 3
                THEN lpad(row_number() OVER (PARTITION BY business_id ORDER BY created_at, id)::text, 3, '0')
                ELSE row_number() OVER (PARTITION BY business_id ORDER BY created_at, id)::text
           END AS new_code
    FROM outlets
)
UPDATE outlets o SET code = ranked.new_code
FROM ranked WHERE ranked.id = o.id;

ALTER TABLE outlets ADD CONSTRAINT outlets_business_id_code_key UNIQUE (business_id, code);
CREATE UNIQUE INDEX outlets_business_id_legacy_code_uq
    ON outlets (business_id, legacy_code) WHERE legacy_code IS NOT NULL;

CREATE TABLE business_outlet_counters (
    business_id BIGINT PRIMARY KEY REFERENCES businesses(id) ON DELETE CASCADE,
    last_allocated BIGINT NOT NULL CHECK (last_allocated >= 1)
);

INSERT INTO business_outlet_counters (business_id, last_allocated)
SELECT business_id, max(code::BIGINT)
FROM outlets
GROUP BY business_id;

ALTER TABLE orders ADD COLUMN outlet_code_snapshot TEXT;
UPDATE orders o
SET outlet_code_snapshot = outlet.legacy_code
FROM outlets outlet
WHERE outlet.business_id = o.business_id AND outlet.id = o.outlet_id;
