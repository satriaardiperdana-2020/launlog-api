ALTER TABLE outlets ADD COLUMN timezone TEXT;

UPDATE outlets SET timezone = 'Asia/Jakarta'
WHERE timezone IS NULL OR btrim(timezone) = '';

ALTER TABLE outlets
    ALTER COLUMN timezone SET DEFAULT 'Asia/Jakarta',
    ALTER COLUMN timezone SET NOT NULL,
    ADD CONSTRAINT outlets_timezone_nonempty CHECK (timezone = btrim(timezone) AND timezone <> '');

COMMENT ON COLUMN outlets.timezone IS 'IANA timezone used by clients to display outlet timestamps; validated by the API. Does not rewrite order instants or change invoice/report day policy.';
