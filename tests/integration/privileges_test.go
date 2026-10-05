//go:build integration

package integration_test

import (
	"context"
	"fmt"
	"net/url"
	"os"
	"testing"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
)

// These cases use disposable roles in the isolated integration database. Never
// run this suite against production. NOINHERIT ensures owner membership is
// rejected even when it does not confer schema or audit privileges directly.
func TestVerifyLeastPrivilegeRejectsElevatedRoles(t *testing.T) {
	if os.Getenv("TEST_EXPECT_LEAST_PRIVILEGE") != "true" {
		t.Skip("requires isolated least-privilege integration setup")
	}
	ctx := context.Background()
	adminURL := os.Getenv("TEST_ADMIN_DATABASE_URL")
	if adminURL == "" {
		t.Fatal("TEST_ADMIN_DATABASE_URL is required")
	}
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	defer admin.Close()
	var databaseName, originalOwner string
	if err := admin.QueryRow(ctx, `SELECT datname, pg_get_userbyid(datdba) FROM pg_database WHERE datname=current_database()`).Scan(&databaseName, &originalOwner); err != nil {
		t.Fatal(err)
	}
	exec := func(t *testing.T, sql string) {
		t.Helper()
		if _, err := admin.Exec(ctx, sql); err != nil {
			t.Fatal(err)
		}
	}
	cases := []struct {
		name, attributes, grant string
		allowed                 bool
	}{
		{name: "restricted", allowed: true},
		{name: "superuser", attributes: "SUPERUSER"},
		{name: "create_role", attributes: "CREATEROLE"},
		{name: "create_database", attributes: "CREATEDB"},
		{name: "create_schema", grant: "GRANT CREATE ON SCHEMA public TO %s"},
		{name: "update_audit", grant: "GRANT UPDATE ON public.audit_logs TO %s"},
		{name: "delete_audit", grant: "GRANT DELETE ON public.audit_logs TO %s"},
		{name: "owner_membership", grant: "GRANT launlog_owner TO %s"},
		{name: "indirect_owner_membership"},
		{name: "database_owner"},
	}
	for i, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			role := fmt.Sprintf("privilege_test_%d_%d", time.Now().UnixNano(), i)
			identifier := pgx.Identifier{role}.Sanitize()
			exec(t, "CREATE ROLE "+identifier+" NOLOGIN NOINHERIT NOSUPERUSER NOCREATEDB NOCREATEROLE")
			t.Cleanup(func() {
				exec(t, "DROP OWNED BY "+identifier)
				exec(t, "DROP ROLE "+identifier)
			})
			if tc.attributes != "" {
				exec(t, "ALTER ROLE "+identifier+" "+tc.attributes)
			}
			exec(t, "GRANT USAGE ON SCHEMA public TO "+identifier)
			exec(t, "GRANT SELECT, INSERT ON public.audit_logs TO "+identifier)
			if tc.grant != "" {
				exec(t, fmt.Sprintf(tc.grant, identifier))
			}
			if tc.name == "indirect_owner_membership" {
				bridge := pgx.Identifier{role + "_bridge"}.Sanitize()
				exec(t, "CREATE ROLE "+bridge+" NOLOGIN NOINHERIT")
				t.Cleanup(func() { exec(t, "DROP ROLE "+bridge) })
				exec(t, "GRANT launlog_owner TO "+bridge)
				exec(t, "GRANT "+bridge+" TO "+identifier)
			}
			if tc.name == "database_owner" {
				db := pgx.Identifier{databaseName}.Sanitize()
				exec(t, "ALTER DATABASE "+db+" OWNER TO "+identifier)
				t.Cleanup(func() { exec(t, "ALTER DATABASE "+db+" OWNER TO "+pgx.Identifier{originalOwner}.Sanitize()) })
			}
			// PostgreSQL applies this role at connection startup. The administrator
			// remains the session user, while privilege checks inspect current_user.
			u, err := url.Parse(adminURL)
			if err != nil {
				t.Fatal("invalid test administrator URL")
			}
			q := u.Query()
			q.Set("role", role)
			u.RawQuery = q.Encode()
			database, err := repository.NewPostgres(ctx, u.String(), "UTC", 5*time.Second, 1, 0)
			if err != nil {
				t.Fatal(err)
			}
			defer database.Close()
			err = database.VerifyLeastPrivilege(ctx)
			if tc.allowed {
				if err != nil {
					t.Fatalf("restricted role rejected: %v", err)
				}
			} else if err == nil || err.Error() != "PostgreSQL runtime role has administrative, schema-creation, or audit-mutation privileges" {
				t.Fatalf("expected privilege rejection, got %v", err)
			}
		})
	}
	t.Run("owner_account", func(t *testing.T) {
		u, err := url.Parse(adminURL)
		if err != nil {
			t.Fatal("invalid test administrator URL")
		}
		q := u.Query()
		q.Set("role", "launlog_owner")
		u.RawQuery = q.Encode()
		database, err := repository.NewPostgres(ctx, u.String(), "UTC", 5*time.Second, 1, 0)
		if err != nil {
			t.Fatal(err)
		}
		defer database.Close()
		if err := database.VerifyLeastPrivilege(ctx); err == nil || err.Error() != "PostgreSQL runtime role has administrative, schema-creation, or audit-mutation privileges" {
			t.Fatalf("expected owner rejection, got %v", err)
		}
	})
}
