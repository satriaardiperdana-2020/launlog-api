ALTER TABLE services ADD COLUMN description TEXT;

COMMENT ON COLUMN services.description IS
    'Optional business-shared service description; changing it does not rewrite historical order-item snapshots.';
