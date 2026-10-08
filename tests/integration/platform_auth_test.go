//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"net/http"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/satriaardiperdana-2020/launlog-api/internal/config"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
	"github.com/satriaardiperdana-2020/launlog-api/internal/server"
)

func TestPlatformSingletonAuthenticationAndImmutableAudit(t *testing.T) {
	if os.Getenv("TEST_ADMIN_DATABASE_URL") == "" {
		t.Skip("TEST_ADMIN_DATABASE_URL is required")
	}
	f := newAuthFixture(t)
	ctx := context.Background()
	t.Cleanup(func() {
		_, _ = f.adminPool.Exec(ctx, "ALTER TABLE platform_audit_logs DISABLE TRIGGER platform_audit_logs_immutable")
		_, _ = f.adminPool.Exec(ctx, "DELETE FROM platform_audit_logs WHERE target_type IN ('session_family','platform_auth') OR action='PLATFORM_ADMIN_BOOTSTRAPPED'")
		_, _ = f.adminPool.Exec(ctx, "ALTER TABLE platform_audit_logs ENABLE TRIGGER platform_audit_logs_immutable")
		_, _ = f.adminPool.Exec(ctx, "DELETE FROM platform_refresh_tokens WHERE platform_admin_id=1")
		_, _ = f.adminPool.Exec(ctx, "DELETE FROM platform_session_families WHERE platform_admin_id=1")
		_, _ = f.adminPool.Exec(ctx, "DELETE FROM platform_admins WHERE id=1")
	})
	hash, err := security.HashPassword("platform-secret-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.adminPool.Exec(ctx, "INSERT INTO platform_admins (id,email,password_hash) VALUES (1,'platform@example.test',$1)", hash); err != nil {
		t.Fatal(err)
	}
	if _, err := f.adminPool.Exec(ctx, "INSERT INTO platform_admins (id,email,password_hash) VALUES (2,'second@example.test',$1)", hash); err == nil {
		t.Fatal("second platform admin was accepted")
	}
	if _, err := f.adminPool.Exec(ctx, "UPDATE platform_admins SET is_active=false WHERE id=1"); err == nil {
		t.Fatal("inactive platform admin was accepted")
	}
	var accountCount int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM platform_admins WHERE is_active").Scan(&accountCount); err != nil || accountCount != 1 {
		t.Fatalf("active platform account count=%d err=%v", accountCount, err)
	}
	platformTokens, err := security.NewPlatformTokenManager("different-platform-secret-at-least-32-bytes", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tenantTokens, err := security.NewTokenManager("integration-test-signing-secret-at-least-32-bytes", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	f.echo = server.New(config.Config{ReadinessTimeout: 2 * time.Second, JWTRefreshTokenTTL: time.Hour}, f.database, tenantTokens, platformTokens)
	if code, _ := f.managementRequest(http.MethodPost, "/platform/auth/login", map[string]string{"email": "missing@example.test", "password": "wrong-password"}, ""); code != 401 {
		t.Fatalf("unknown platform login=%d", code)
	}
	code, body := f.managementRequest(http.MethodPost, "/platform/auth/login", map[string]string{"email": "platform@example.test", "password": "platform-secret-password"}, "")
	if code != 200 {
		t.Fatalf("platform login=%d body=%s", code, body)
	}
	var first sessionPayload
	if err := json.Unmarshal(body, &first); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.managementRequest(http.MethodGet, "/platform/auth/me", nil, first.Tokens.AccessToken); code != 200 {
		t.Fatalf("platform identity=%d", code)
	}
	if code, _ := f.managementRequest(http.MethodGet, "/outlets", nil, first.Tokens.AccessToken); code != 401 {
		t.Fatalf("tenant route accepted platform token: %d", code)
	}
	tenantBearer := f.ownerLogin(t)
	if code, _ := f.managementRequest(http.MethodGet, "/platform/auth/me", nil, tenantBearer); code != 401 {
		t.Fatalf("platform route accepted tenant token: %d", code)
	}
	code, body = f.managementRequest(http.MethodPost, "/platform/auth/refresh", map[string]string{"refreshToken": first.Tokens.RefreshToken}, "")
	if code != 200 {
		t.Fatalf("platform refresh=%d body=%s", code, body)
	}
	var second sessionPayload
	if err := json.Unmarshal(body, &second); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.managementRequest(http.MethodPost, "/platform/auth/refresh", map[string]string{"refreshToken": first.Tokens.RefreshToken}, ""); code != 401 {
		t.Fatalf("platform replay=%d", code)
	}
	if code, _ := f.managementRequest(http.MethodGet, "/platform/auth/me", nil, second.Tokens.AccessToken); code != 401 {
		t.Fatalf("replayed session remained active: %d", code)
	}
	// A database audit failure must roll back session creation and return no tokens.
	var sessionsBefore int
	var count int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM platform_session_families").Scan(&sessionsBefore); err != nil {
		t.Fatal(err)
	}
	if _, err := f.adminPool.Exec(ctx, `CREATE FUNCTION reject_platform_test_audit() RETURNS trigger LANGUAGE plpgsql AS $$
		BEGIN RAISE EXCEPTION 'test audit rejection'; END; $$`); err != nil {
		t.Fatal(err)
	}
	if _, err := f.adminPool.Exec(ctx, `CREATE TRIGGER reject_platform_test_audit BEFORE INSERT ON platform_audit_logs
		FOR EACH ROW EXECUTE FUNCTION reject_platform_test_audit()`); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		_, _ = f.adminPool.Exec(ctx, "DROP TRIGGER IF EXISTS reject_platform_test_audit ON platform_audit_logs")
		_, _ = f.adminPool.Exec(ctx, "DROP FUNCTION IF EXISTS reject_platform_test_audit()")
	})
	if code, _ := f.managementRequest(http.MethodPost, "/platform/auth/login", map[string]string{"email": "platform@example.test", "password": "platform-secret-password"}, ""); code != 500 {
		t.Fatalf("audit failure login status=%d, want 500", code)
	}
	if _, err := f.adminPool.Exec(ctx, "DROP TRIGGER reject_platform_test_audit ON platform_audit_logs"); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM platform_session_families").Scan(&count); err != nil || count != sessionsBefore {
		t.Fatalf("audit failure left session family: before=%d after=%d err=%v", sessionsBefore, count, err)
	}
	var auditText string
	if err := f.pool.QueryRow(ctx, "SELECT count(*), coalesce(string_agg(metadata::text,' '),'') FROM platform_audit_logs").Scan(&count, &auditText); err != nil || count != 4 {
		t.Fatalf("platform session audit count=%d err=%v", count, err)
	}
	if strings.Contains(auditText, first.Tokens.RefreshToken) || strings.Contains(auditText, hash) {
		t.Fatal("platform audit contains a credential")
	}
	if _, err := f.adminPool.Exec(ctx, "UPDATE platform_audit_logs SET action='tampered' WHERE action='PLATFORM_SESSION_CREATED'"); err == nil {
		t.Fatal("platform audit UPDATE was accepted")
	}
	if _, err := f.adminPool.Exec(ctx, "DELETE FROM platform_audit_logs WHERE action='PLATFORM_SESSION_CREATED'"); err == nil {
		t.Fatal("platform audit DELETE was accepted")
	}
}
