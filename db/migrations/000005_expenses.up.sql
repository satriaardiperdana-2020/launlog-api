CREATE TABLE expense_categories (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL REFERENCES businesses (id),
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    description TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (business_id, id)
);

CREATE UNIQUE INDEX expense_categories_business_name_uq
    ON expense_categories (business_id, lower(name));

CREATE INDEX expense_categories_business_active_idx
    ON expense_categories (business_id, is_active);

CREATE TABLE expenses (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL,
    outlet_id BIGINT NOT NULL,
    category_id BIGINT NOT NULL,
    amount BIGINT NOT NULL CHECK (amount > 0),
    description TEXT NOT NULL CHECK (btrim(description) <> ''),
    expense_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    receipt_reference TEXT,
    created_by BIGINT NOT NULL,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (business_id, outlet_id, id),
    FOREIGN KEY (business_id, outlet_id)
        REFERENCES outlets (business_id, id),
    FOREIGN KEY (business_id, category_id)
        REFERENCES expense_categories (business_id, id),
    FOREIGN KEY (business_id, created_by)
        REFERENCES users (business_id, id)
);

CREATE INDEX expenses_outlet_date_idx
    ON expenses (business_id, outlet_id, expense_at DESC);

CREATE INDEX expenses_category_date_idx
    ON expenses (business_id, category_id, expense_at DESC);

CREATE INDEX expenses_creator_date_idx
    ON expenses (business_id, created_by, expense_at DESC);
