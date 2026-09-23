#!/usr/bin/env bash
set -euo pipefail

: "${UPGRADE_MIGRATION_DATABASE_URL:?must point to a new disposable upgrade-test database as the migration role}"
: "${UPGRADE_ADMIN_DATABASE_URL:?must point to that same isolated database as a test administrator}"
: "${MIGRATE:?must point to the pinned golang-migrate executable}"

if psql "$UPGRADE_ADMIN_DATABASE_URL" -Atqc "SELECT to_regclass('public.schema_migrations') IS NOT NULL OR EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m','S'))" | grep -qx t; then
	echo 'upgrade-test database is not empty; refusing to overwrite it' >&2
	exit 1
fi

"$MIGRATE" -path db/migrations -database "$UPGRADE_MIGRATION_DATABASE_URL" up 6

psql "$UPGRADE_ADMIN_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
INSERT INTO businesses (id, name) VALUES (-916001, 'Upgrade rehearsal business');
INSERT INTO outlets (id, business_id, code, name) VALUES (-916002, -916001, 'UPG', 'Upgrade rehearsal outlet');
INSERT INTO users (id, business_id, email, full_name, password_hash, role)
VALUES (-916003, -916001, 'upgrade@example.test', 'Upgrade user', 'test-only-hash', 'ADMIN');
INSERT INTO user_outlets (business_id, user_id, outlet_id) VALUES (-916001, -916003, -916002);
INSERT INTO customers (id, business_id, name, phone, address)
VALUES (-916004, -916001, 'Legacy Customer Snapshot', '+628111111111', 'Legacy address');
INSERT INTO orders (id, business_id, outlet_id, customer_id, invoice_number, total_amount, created_by)
VALUES (-916005, -916001, -916002, -916004, 'UPG-LEGACY-000001', 12500, -916003);
INSERT INTO refresh_tokens (id, business_id, user_id, token_hash, expires_at)
VALUES (-916006, -916001, -916003, 'upgrade-rehearsal-legacy-refresh-hash', now() + interval '1 day');
SQL

"$MIGRATE" -path db/migrations -database "$UPGRADE_MIGRATION_DATABASE_URL" up

psql "$UPGRADE_ADMIN_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
DO $$
DECLARE
  snapshot_name text;
  snapshot_phone text;
  token_family_id bigint;
  current_version bigint;
  migration_dirty boolean;
BEGIN
  SELECT customer_name_snapshot, customer_phone_snapshot
    INTO snapshot_name, snapshot_phone FROM orders WHERE id = -916005;
  IF snapshot_name <> 'Legacy Customer Snapshot' OR snapshot_phone <> '+628111111111' THEN
    RAISE EXCEPTION 'legacy order customer snapshot was not backfilled';
  END IF;
  SELECT family_id INTO token_family_id FROM refresh_tokens WHERE id = -916006
    AND token_hash = 'upgrade-rehearsal-legacy-refresh-hash';
  IF token_family_id IS NULL THEN
    RAISE EXCEPTION 'legacy refresh session/hash was not preserved and attached to a family';
  END IF;
  SELECT version, dirty INTO current_version, migration_dirty FROM schema_migrations;
  IF current_version <> 16 OR migration_dirty THEN
    RAISE EXCEPTION 'upgrade did not finish cleanly at migration 16 (version %, dirty %)', current_version, migration_dirty;
  END IF;
END $$;
SQL

echo 'Migration upgrade rehearsal passed: legacy business data, order snapshots, and refresh-token hashes were preserved.'
