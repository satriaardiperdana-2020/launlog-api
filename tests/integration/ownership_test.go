//go:build integration

package integration_test

import (
	"context"
	"errors"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgxpool"
)

func TestOwnershipMigrationAndTenantIsolation(t *testing.T) {
	databaseURL := os.Getenv("TEST_DATABASE_URL")
	if databaseURL == "" {
		t.Skip("TEST_DATABASE_URL is required for PostgreSQL integration tests")
	}

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		t.Fatalf("connect integration database: %v", err)
	}
	defer pool.Close()

	tx, err := pool.Begin(ctx)
	if err != nil {
		t.Fatalf("begin transaction: %v", err)
	}
	defer func() {
		if err := tx.Rollback(ctx); err != nil && !errors.Is(err, pgx.ErrTxClosed) {
			t.Errorf("rollback integration transaction: %v", err)
		}
	}()

	assertSchemaVersion(t, ctx, tx)

	const (
		businessAID = int64(-10001)
		businessBID = int64(-10002)
		userAID     = int64(-10011)
		userBID     = int64(-10012)
		outletAID   = int64(-10021)
		outletBID   = int64(-10022)
	)

	mustExec(t, ctx, tx, `INSERT INTO businesses (id, name) VALUES ($1, 'Ownership Test A'), ($2, 'Ownership Test B')`, businessAID, businessBID)
	mustExec(t, ctx, tx, `INSERT INTO users (id, business_id, email, full_name, password_hash, role) VALUES
		($1, $2, 'ownership-a@example.test', 'Ownership A', 'hash', 'ADMIN'),
		($3, $4, 'ownership-b@example.test', 'Ownership B', 'hash', 'LAUNDRY_STAFF')`, userAID, businessAID, userBID, businessBID)
	mustExec(t, ctx, tx, `INSERT INTO outlets (id, business_id, code, name) VALUES
		($1, $2, 'A', 'Outlet A'),
		($3, $4, 'B', 'Outlet B')`, outletAID, businessAID, outletBID, businessBID)

	mustExec(t, ctx, tx, `INSERT INTO user_outlets (business_id, user_id, outlet_id) VALUES ($1, $2, $3)`, businessAID, userAID, outletAID)
	assertForeignKeyViolation(t, mustExecErr(ctx, tx,
		`INSERT INTO user_outlets (business_id, user_id, outlet_id) VALUES ($1, $2, $3)`, businessAID, userAID, outletBID))

	mustExec(t, ctx, tx, `INSERT INTO session_families (id, business_id, user_id) VALUES (-10031, $1, $2)`, businessAID, userAID)
	mustExec(t, ctx, tx, `INSERT INTO refresh_tokens (id, business_id, user_id, family_id, token_hash, expires_at) VALUES
		(-10031, $1, $2, -10031, 'ownership-test-token-a', now() + interval '1 hour')`, businessAID, userAID)
	assertForeignKeyViolation(t, mustExecErr(ctx, tx,
		`INSERT INTO refresh_tokens (id, business_id, user_id, family_id, token_hash, expires_at) VALUES
		(-10032, $1, $2, -10031, 'ownership-test-token-cross-business', now() + interval '1 hour')`, businessBID, userAID))

	mustExec(t, ctx, tx, `INSERT INTO services (id, business_id, name, unit, unit_price_amount) VALUES
		(-10041, $1, 'Wash', 'KILOGRAM', 7000),
		(-10042, $2, 'Wash', 'KILOGRAM', 7000)`, businessAID, businessBID)
	assertCheckViolation(t, mustExecErr(ctx, tx,
		`INSERT INTO services (id, business_id, name, unit, unit_price_amount) VALUES (-10043, $1, 'Invalid', 'LITRE', 7000)`, businessAID))

	var serviceOutletColumnExists bool
	if err := tx.QueryRow(ctx, `
		SELECT EXISTS (
			SELECT 1
			FROM information_schema.columns
			WHERE table_schema = 'public' AND table_name = 'services' AND column_name = 'outlet_id'
		)`).Scan(&serviceOutletColumnExists); err != nil {
		t.Fatalf("inspect services scope: %v", err)
	}
	if serviceOutletColumnExists {
		t.Fatal("services must remain business-shared; an outlet_id would conflict with the documented ownership model")
	}

	var serviceBusinessComment, unitComment string
	if err := tx.QueryRow(ctx, `SELECT col_description('services'::regclass, 2), col_description('services'::regclass, 4)`).Scan(&serviceBusinessComment, &unitComment); err != nil {
		t.Fatalf("read services comments: %v", err)
	}
	if serviceBusinessComment == "" || unitComment == "" {
		t.Fatal("ownership migration must document services.business_id and services.unit")
	}

	var sessionIndexExists bool
	if err := tx.QueryRow(ctx, `SELECT to_regclass('public.refresh_tokens_business_user_idx') IS NOT NULL`).Scan(&sessionIndexExists); err != nil {
		t.Fatalf("inspect refresh-token index: %v", err)
	}
	if !sessionIndexExists {
		t.Fatal("refresh_tokens_business_user_idx is required for tenant-aware session lookups")
	}
}

func assertSchemaVersion(t *testing.T, ctx context.Context, tx pgx.Tx) {
	t.Helper()

	var version int64
	var dirty bool
	if err := tx.QueryRow(ctx, `SELECT version, dirty FROM schema_migrations`).Scan(&version, &dirty); err != nil {
		t.Fatalf("read schema migration state: %v", err)
	}
	if version != 8 || dirty {
		t.Fatalf("expected clean session-family migration at version 8, got version=%d dirty=%t", version, dirty)
	}
}

func mustExec(t *testing.T, ctx context.Context, tx pgx.Tx, query string, args ...any) {
	t.Helper()
	if _, err := tx.Exec(ctx, query, args...); err != nil {
		t.Fatalf("execute statement: %v", err)
	}
}

func mustExecErr(ctx context.Context, tx pgx.Tx, query string, args ...any) error {
	if _, err := tx.Exec(ctx, "SAVEPOINT expected_constraint_error"); err != nil {
		return err
	}

	_, statementErr := tx.Exec(ctx, query, args...)
	if _, err := tx.Exec(ctx, "ROLLBACK TO SAVEPOINT expected_constraint_error"); err != nil {
		return err
	}
	if _, err := tx.Exec(ctx, "RELEASE SAVEPOINT expected_constraint_error"); err != nil {
		return err
	}

	return statementErr
}

func assertForeignKeyViolation(t *testing.T, err error) {
	t.Helper()
	assertSQLState(t, err, "23503")
}

func assertCheckViolation(t *testing.T, err error) {
	t.Helper()
	assertSQLState(t, err, "23514")
}

func assertSQLState(t *testing.T, err error, code string) {
	t.Helper()
	if err == nil {
		t.Fatalf("expected PostgreSQL error %s, got nil", code)
	}
	var pgErr *pgconn.PgError
	if !errors.As(err, &pgErr) || pgErr.Code != code {
		t.Fatalf("expected PostgreSQL error %s, got %v", code, err)
	}
}
