#!/usr/bin/env bash
set -euo pipefail

: "${BACKUP_SOURCE_DATABASE_URL:?must point to an isolated source database}"
: "${RESTORE_DATABASE_URL:?must point to a separate disposable empty restore database}"

tmp="$(mktemp -d)"
trap 'rm -rf "$tmp"' EXIT
dump_file="$tmp/launlog.dump"

psql "$BACKUP_SOURCE_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
DO $$
DECLARE v bigint; d boolean;
BEGIN
  SELECT version, dirty INTO v,d FROM schema_migrations;
  IF v <> 16 OR d THEN RAISE EXCEPTION 'source schema is not clean at version 16'; END IF;
END $$;
SQL

if psql "$RESTORE_DATABASE_URL" -Atqc "SELECT to_regclass('public.schema_migrations') IS NOT NULL OR EXISTS (SELECT 1 FROM pg_class c JOIN pg_namespace n ON n.oid=c.relnamespace WHERE n.nspname='public' AND c.relkind IN ('r','p','v','m','S'))" | grep -qx t; then
	echo 'restore target is not empty; refusing to overwrite existing database objects' >&2
	exit 1
fi

pg_dump --format=custom --no-owner --no-privileges --file="$dump_file" "$BACKUP_SOURCE_DATABASE_URL"
pg_restore --exit-on-error --no-owner --no-privileges --dbname="$RESTORE_DATABASE_URL" "$dump_file"

psql "$RESTORE_DATABASE_URL" -v ON_ERROR_STOP=1 <<'SQL'
DO $$
DECLARE
  current_version bigint;
  migration_dirty boolean;
  required_tables integer;
BEGIN
  SELECT version, dirty INTO current_version, migration_dirty FROM schema_migrations;
  IF current_version <> 16 OR migration_dirty THEN
    RAISE EXCEPTION 'restored database schema version/dirty state mismatch';
  END IF;
  SELECT count(*) INTO required_tables FROM information_schema.tables
    WHERE table_schema = 'public' AND table_type = 'BASE TABLE'
      AND table_name IN ('businesses','users','customers','orders','order_items','payments','expenses','audit_logs');
  IF required_tables <> 8 THEN
    RAISE EXCEPTION 'restore is missing required application tables';
  END IF;
END $$;
SQL

echo 'Backup/restore rehearsal passed: custom-format dump restored into the separate disposable database and schema verified.'
