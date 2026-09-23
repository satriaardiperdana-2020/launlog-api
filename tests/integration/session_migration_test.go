//go:build integration

package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestSessionFamilyMigrationPreservesLegacySessions(t *testing.T) {
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	defer pool.Close()
	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer tx.Rollback(ctx)

	// Isolate the exact migration scripts in a transaction-local schema.
	// The shared CI database and its schema_migrations row are untouched.
	schema := "issue003_backfill_" + time.Now().Format("150405000000000")
	if _, err := tx.Exec(ctx, "CREATE SCHEMA "+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, "SET LOCAL search_path TO "+schema); err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, `CREATE TABLE users (id BIGINT, business_id BIGINT, UNIQUE (business_id,id));
		CREATE TABLE refresh_tokens (
			id BIGINT PRIMARY KEY, business_id BIGINT NOT NULL, user_id BIGINT NOT NULL,
			token_hash TEXT NOT NULL UNIQUE, created_at TIMESTAMPTZ NOT NULL,
			expires_at TIMESTAMPTZ NOT NULL, revoked_at TIMESTAMPTZ
		);
		INSERT INTO users VALUES (21, 11);
		INSERT INTO refresh_tokens VALUES
			(7, 11, 21, 'legacy-active-hash', now(), now()+interval '1 hour', NULL),
			(-8, 11, 21, 'legacy-consumed-hash', now(), now()+interval '1 hour', now());`); err != nil {
		t.Fatal(err)
	}

	up, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000008_session_families.up.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(up)); err != nil {
		t.Fatalf("apply legacy backfill: %v", err)
	}
	var count int
	if err := tx.QueryRow(ctx, `SELECT count(*) FROM refresh_tokens rt
		JOIN session_families sf ON sf.id=rt.family_id AND sf.business_id=rt.business_id
		WHERE rt.token_hash IN ('legacy-active-hash','legacy-consumed-hash')
		AND rt.id=rt.family_id`).Scan(&count); err != nil || count != 2 {
		t.Fatalf("legacy hashes/families not preserved: count=%d err=%v", count, err)
	}
	var newID int64
	if err := tx.QueryRow(ctx, "INSERT INTO session_families (business_id,user_id) VALUES (11,21) RETURNING id").Scan(&newID); err != nil || newID <= 7 {
		t.Fatalf("new family sequence was not advanced: id=%d err=%v", newID, err)
	}
	down, err := os.ReadFile(filepath.Join("..", "..", "db", "migrations", "000008_session_families.down.sql"))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := tx.Exec(ctx, string(down)); err != nil {
		t.Fatalf("roll back family schema: %v", err)
	}
	if err := tx.QueryRow(ctx, "SELECT count(*) FROM refresh_tokens WHERE token_hash LIKE 'legacy-%'").Scan(&count); err != nil || count != 2 {
		t.Fatalf("rollback lost legacy sessions: count=%d err=%v", count, err)
	}
}
