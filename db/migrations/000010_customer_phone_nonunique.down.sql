-- Restores the former constraint. This intentionally fails if duplicate active
-- phone numbers were recorded after migration 000010 was applied.
CREATE UNIQUE INDEX customers_business_phone_uq
    ON customers (business_id, phone)
    WHERE phone IS NOT NULL AND deleted_at IS NULL;
