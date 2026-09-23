CREATE TABLE receipt_templates (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL,
    outlet_id BIGINT NOT NULL,
    name TEXT NOT NULL CHECK (btrim(name) <> ''),
    header_text TEXT,
    footer_text TEXT,
    whatsapp_message_template TEXT,
    paper_width_mm INTEGER NOT NULL DEFAULT 58 CHECK (paper_width_mm IN (58, 80)),
    is_default BOOLEAN NOT NULL DEFAULT FALSE,
    created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    deleted_at TIMESTAMPTZ,
    UNIQUE (business_id, outlet_id, id),
    FOREIGN KEY (business_id, outlet_id)
        REFERENCES outlets (business_id, id)
);

CREATE UNIQUE INDEX receipt_templates_outlet_name_uq
    ON receipt_templates (business_id, outlet_id, lower(name))
    WHERE deleted_at IS NULL;

CREATE UNIQUE INDEX receipt_templates_one_default_uq
    ON receipt_templates (business_id, outlet_id)
    WHERE is_default AND deleted_at IS NULL;

CREATE TABLE audit_logs (
    id BIGSERIAL PRIMARY KEY,
    business_id BIGINT NOT NULL REFERENCES businesses (id),
    outlet_id BIGINT,
    actor_user_id BIGINT,
    action TEXT NOT NULL CHECK (btrim(action) <> ''),
    entity_type TEXT NOT NULL CHECK (btrim(entity_type) <> ''),
    entity_id BIGINT,
    old_values JSONB,
    new_values JSONB,
    ip_address INET,
    user_agent TEXT,
    occurred_at TIMESTAMPTZ NOT NULL DEFAULT now(),
    FOREIGN KEY (business_id, outlet_id)
        REFERENCES outlets (business_id, id),
    FOREIGN KEY (business_id, actor_user_id)
        REFERENCES users (business_id, id),
    CHECK (old_values IS NULL OR jsonb_typeof(old_values) = 'object'),
    CHECK (new_values IS NULL OR jsonb_typeof(new_values) = 'object')
);

CREATE INDEX audit_logs_business_date_idx
    ON audit_logs (business_id, occurred_at DESC);

CREATE INDEX audit_logs_outlet_date_idx
    ON audit_logs (business_id, outlet_id, occurred_at DESC)
    WHERE outlet_id IS NOT NULL;

CREATE INDEX audit_logs_actor_date_idx
    ON audit_logs (business_id, actor_user_id, occurred_at DESC)
    WHERE actor_user_id IS NOT NULL;

CREATE INDEX audit_logs_entity_idx
    ON audit_logs (business_id, entity_type, entity_id, occurred_at DESC);

CREATE FUNCTION prevent_audit_log_mutation()
RETURNS TRIGGER
LANGUAGE plpgsql
AS $$
BEGIN
    RAISE EXCEPTION 'audit logs are immutable';
END;
$$;

CREATE TRIGGER audit_logs_immutable
    BEFORE UPDATE OR DELETE ON audit_logs
    FOR EACH ROW
    EXECUTE FUNCTION prevent_audit_log_mutation();
