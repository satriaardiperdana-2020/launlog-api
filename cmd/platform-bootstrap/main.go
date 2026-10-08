package main

import (
	"bufio"
	"context"
	"errors"
	"flag"
	"fmt"
	"io"
	"net/mail"
	"net/url"
	"os"
	"strings"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
)

func main() {
	if err := run(); err != nil {
		fmt.Fprintln(os.Stderr, "platform bootstrap failed:", err)
		os.Exit(1)
	}
	fmt.Fprintln(os.Stdout, "platform admin created")
}

func run() error {
	emailFlag := flag.String("email", "", "platform administrator email (never supply a password argument)")
	flag.Parse()
	if flag.NArg() != 0 {
		return errors.New("unexpected arguments")
	}
	email := strings.ToLower(strings.TrimSpace(*emailFlag))
	parsed, err := mail.ParseAddress(email)
	if err != nil || parsed.Address != email || len(email) > 254 {
		return errors.New("valid --email is required")
	}
	databaseURL := os.Getenv("PLATFORM_BOOTSTRAP_DATABASE_URL")
	if databaseURL == "" {
		return errors.New("PLATFORM_BOOTSTRAP_DATABASE_URL is required")
	}
	if strings.EqualFold(os.Getenv("APP_ENV"), "production") {
		parsedURL, err := url.Parse(databaseURL)
		if err != nil || parsedURL.Query().Get("sslmode") != "verify-full" {
			return errors.New("production PLATFORM_BOOTSTRAP_DATABASE_URL must use sslmode=verify-full")
		}
	}
	// Descriptor 3 comes from the operator's secret manager. Passwords are not
	// accepted through argv, environment variables, or an echoed terminal.
	input := os.NewFile(3, "bootstrap-password")
	if input == nil {
		return errors.New("password input descriptor 3 is required")
	}
	defer input.Close()
	passwordLine, err := bufio.NewReader(io.LimitReader(input, 1024)).ReadString('\n')
	if err != nil && !errors.Is(err, io.EOF) {
		return errors.New("cannot read password descriptor 3")
	}
	password := strings.TrimSuffix(strings.TrimSuffix(passwordLine, "\n"), "\r")
	hash, err := security.HashPassword(password)
	if err != nil {
		return errors.New("password does not meet the approved password policy")
	}
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
	defer cancel()
	pool, err := pgxpool.New(ctx, databaseURL)
	if err != nil {
		return errors.New("cannot connect to bootstrap database")
	}
	defer pool.Close()
	var role string
	var canCreateRole, canCreateDB, runtimeMember, canDeleteAudit, canTruncateAudit bool
	var isSuperuser, canCreateSchema, ownerMember, canUpdateAudit bool
	if err := pool.QueryRow(ctx, `SELECT r.rolcreaterole,r.rolcreatedb,pg_has_role(current_user, 'launlog_runtime', 'MEMBER'),has_table_privilege(current_user, 'public.platform_audit_logs', 'DELETE'),has_table_privilege(current_user, 'public.platform_audit_logs', 'TRUNCATE'),current_user, r.rolsuper,
		has_schema_privilege(current_user, 'public', 'CREATE'),
		pg_has_role(current_user, 'launlog_owner', 'MEMBER'),
		has_table_privilege(current_user, 'public.platform_audit_logs', 'UPDATE')
		FROM pg_roles r WHERE r.rolname=current_user`).Scan(&canCreateRole, &canCreateDB, &runtimeMember, &canDeleteAudit, &canTruncateAudit, &role, &isSuperuser, &canCreateSchema, &ownerMember, &canUpdateAudit); err != nil || canCreateRole || canCreateDB || runtimeMember || canDeleteAudit || canTruncateAudit || role != "launlog_bootstrap" || isSuperuser || canCreateSchema || ownerMember || canUpdateAudit {
		return errors.New("dedicated least-privilege launlog_bootstrap role is required")
	}
	tx, err := pool.Begin(ctx)
	if err != nil {
		return errors.New("cannot begin bootstrap transaction")
	}
	defer tx.Rollback(ctx)
	if _, err := tx.Exec(ctx, `INSERT INTO platform_admins (id, email, password_hash) VALUES (1, $1, $2)`, email, hash); err != nil {
		return errors.New("platform admin already exists or bootstrap insert was denied")
	}
	if _, err := tx.Exec(ctx, `INSERT INTO platform_audit_logs (action, target_type, target_id, outcome)
		VALUES ('PLATFORM_ADMIN_BOOTSTRAPPED', 'platform_admin', 1, 'SUCCESS')`); err != nil {
		return errors.New("cannot write bootstrap audit")
	}
	if err := tx.Commit(ctx); err != nil {
		return errors.New("cannot commit bootstrap transaction")
	}
	return nil
}
