CREATE TABLE customers (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL REFERENCES businesses (id),
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    phone TEXT,
    email TEXT,
    address TEXT,
    notes TEXT,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (business_id, id)
);

CREATE INDEX customers_business_name_idx
    ON customers (business_id, lower(name));

CREATE UNIQUE INDEX customers_business_phone_uq
    ON customers (business_id, phone)
    WHERE phone IS NOT NULL AND deleted_at IS NULL;

CREATE INDEX customers_business_created_idx
    ON customers (business_id, created_at DESC);

CREATE TABLE services (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL REFERENCES businesses (id),
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    unit TEXT NOT NULL CHECK (unit IN ('KILOGRAM', 'PIECE', 'METER', 'SQUARE_METER')),
    unit_price_amount BIGINT NOT NULL CHECK (unit_price_amount >= 0),
    estimated_duration_minutes INTEGER NOT NULL DEFAULT 0
        CHECK (estimated_duration_minutes >= 0),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (business_id, id)
);

CREATE UNIQUE INDEX services_business_name_unit_uq
    ON services (business_id, lower(name), unit)
    WHERE deleted_at IS NULL;

CREATE INDEX services_business_active_idx
    ON services (business_id, is_active)
    WHERE deleted_at IS NULL;

CREATE TABLE perfumes (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL REFERENCES businesses (id),
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (business_id, id)
);

CREATE UNIQUE INDEX perfumes_business_name_uq
    ON perfumes (business_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE INDEX perfumes_business_active_idx
    ON perfumes (business_id, is_active)
    WHERE deleted_at IS NULL;
