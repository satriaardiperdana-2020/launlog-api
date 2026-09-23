CREATE TABLE businesses (
    id BIGSERIAL PRIMARY KEY,
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    timezone TEXT NOT NULL DEFAULT 'Asia/Jakarta' CHECK (timezone = 'Asia/Jakarta'),
    phone TEXT,
    address TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE outlets (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL REFERENCES businesses (id),
    code TEXT NOT NULL CHECK (btrim(code) <> ''),
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    phone TEXT,
    address TEXT,
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (business_id, id),
    UNIQUE (business_id, code)
);

CREATE INDEX outlets_business_active_idx
    ON outlets (business_id, is_active);

CREATE TABLE users (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL REFERENCES businesses (id),
    email TEXT NOT NULL CHECK (btrim(email) <> ''),
    full_name TEXT NOT NULL CHECK (btrim(full_name) <> ''),
    password_hash TEXT NOT NULL CHECK (btrim(password_hash) <> ''),
    role TEXT NOT NULL CHECK (role IN ('ADMIN', 'LAUNDRY_STAFF')),
    is_active BOOLEAN NOT NULL DEFAULT TRUE,
    last_login_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (business_id, id)
);

CREATE UNIQUE INDEX users_email_uq
    ON users (lower(email));

CREATE INDEX users_business_role_active_idx
    ON users (business_id, role, is_active);

CREATE TABLE user_outlets (
    business_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    outlet_id BIGINT NOT NULL,
    assigned_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, user_id, outlet_id),
    FOREIGN KEY (business_id, user_id)
        REFERENCES users (business_id, id),
    FOREIGN KEY (business_id, outlet_id)
        REFERENCES outlets (business_id, id)
);

CREATE INDEX user_outlets_outlet_idx
    ON user_outlets (business_id, outlet_id, user_id);

CREATE TABLE permissions (
    code TEXT PRIMARY KEY CHECK (code ~ '^[A-Z][A-Z0-9_]*$'),
    description TEXT NOT NULL CHECK (btrim(description) <> ''),
    created_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE user_permissions (
    business_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    permission_code TEXT NOT NULL REFERENCES permissions (code),
    granted_by BIGINT NOT NULL,
    granted_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (business_id, user_id, permission_code),
    FOREIGN KEY (business_id, user_id)
        REFERENCES users (business_id, id),
    FOREIGN KEY (business_id, granted_by)
        REFERENCES users (business_id, id)
);

CREATE INDEX user_permissions_permission_idx
    ON user_permissions (permission_code, business_id);

CREATE TABLE refresh_tokens (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL,
    user_id BIGINT NOT NULL,
    token_hash TEXT NOT NULL UNIQUE CHECK (btrim(token_hash) <> ''),
    expires_at TIMESTAMPTZ NOT NULL,
    revoked_at TIMESTAMPTZ,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    UNIQUE (business_id, id),
    FOREIGN KEY (business_id, user_id)
        REFERENCES users (business_id, id),
    CHECK (expires_at > created_at),
    CHECK (revoked_at IS NULL OR revoked_at >= created_at)
);

CREATE INDEX refresh_tokens_user_active_idx
    ON refresh_tokens (business_id, user_id, expires_at)
    WHERE revoked_at IS NULL;
