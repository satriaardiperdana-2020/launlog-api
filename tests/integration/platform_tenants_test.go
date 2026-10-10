//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/satriaardiperdana-2020/launlog-api/internal/config"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
	"github.com/satriaardiperdana-2020/launlog-api/internal/server"
)

type platformTenantFixture struct {
	f           *authFixture
	token       string
	businessIDs []int64
	suffix      string
}
type provisionedTenant struct {
	Business struct {
		ID int64 `json:"id"`
	} `json:"business"`
	Outlet struct {
		ID   int64  `json:"id"`
		Code string `json:"code"`
	} `json:"firstOutlet"`
	Admin struct {
		ID   int64  `json:"id"`
		Role string `json:"role"`
	} `json:"firstAdmin"`
	email, token string
	perfume      int64
}

func newPlatformTenantFixture(t *testing.T) *platformTenantFixture {
	t.Helper()
	for _, key := range []string{"TEST_DATABASE_URL", "TEST_ADMIN_DATABASE_URL", "TEST_BOOTSTRAP_DATABASE_URL"} {
		if os.Getenv(key) == "" {
			t.Fatalf("%s required; refusing to skip", key)
		}
	}
	if os.Getenv("TEST_EXPECT_LEAST_PRIVILEGE") != "true" {
		t.Fatal("restricted integration roles required")
	}
	f := newAuthFixture(t)
	p := &platformTenantFixture{f: f, suffix: fmt.Sprint(time.Now().UnixNano())}
	ctx := context.Background()
	t.Cleanup(func() {
		exec := func(query string, args ...any) {
			if _, err := f.adminPool.Exec(ctx, query, args...); err != nil {
				t.Errorf("cleanup: %v", err)
			}
		}
		exec("ALTER TABLE audit_logs DISABLE TRIGGER audit_logs_immutable")
		exec("ALTER TABLE platform_audit_logs DISABLE TRIGGER platform_audit_logs_immutable")
		exec("DELETE FROM audit_logs WHERE business_id=ANY($1)", p.businessIDs)
		exec("DELETE FROM platform_audit_logs WHERE actor_platform_admin_id=1 OR business_id=ANY($1)", p.businessIDs)
		exec("DELETE FROM platform_support_sessions WHERE business_id=ANY($1)", p.businessIDs)
		exec("DELETE FROM platform_support_requests WHERE business_id=ANY($1)", p.businessIDs)
		for _, table := range []string{"perfumes", "refresh_tokens", "session_families", "user_outlets", "users", "outlets"} {
			exec("DELETE FROM "+table+" WHERE business_id=ANY($1)", p.businessIDs)
		}
		exec("DELETE FROM businesses WHERE id=ANY($1)", p.businessIDs)
		exec("DELETE FROM platform_refresh_tokens WHERE platform_admin_id=1")
		exec("DELETE FROM platform_session_families WHERE platform_admin_id=1")
		exec("DELETE FROM platform_admins WHERE id=1")
		exec("ALTER TABLE audit_logs ENABLE TRIGGER audit_logs_immutable")
		exec("ALTER TABLE platform_audit_logs ENABLE TRIGGER platform_audit_logs_immutable")
	})
	hash, err := security.HashPassword("platform-secret-password")
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.adminPool.Exec(ctx, "INSERT INTO platform_admins(id,email,password_hash) VALUES(1,'platform@example.test',$1)", hash); err != nil {
		t.Fatal(err)
	}
	pt, err := security.NewPlatformTokenManager("different-platform-secret-at-least-32-bytes", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	tt, err := security.NewTokenManager("integration-test-signing-secret-at-least-32-bytes", 10*time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	f.echo = server.New(config.Config{JWTRefreshTokenTTL: time.Hour}, f.database, tt, pt)
	body := p.expect(t, http.MethodPost, "/platform/auth/login", map[string]any{"email": "platform@example.test", "password": "platform-secret-password"}, "", 200, "")
	var login sessionPayload
	if err := json.Unmarshal(body, &login); err != nil {
		t.Fatal(err)
	}
	p.token = login.Tokens.AccessToken
	return p
}
func (p *platformTenantFixture) request(method, path string, input any, token, etag string) (int, []byte) {
	encoded, _ := json.Marshal(input)
	req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("X-Request-ID", "issue018-review")
	if etag != "" {
		req.Header.Set("If-Match", etag)
	}
	rec := httptest.NewRecorder()
	p.f.echo.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}
func (p *platformTenantFixture) expect(t *testing.T, method, path string, input any, token string, want int, etag string) []byte {
	t.Helper()
	code, body := p.request(method, path, input, token, etag)
	if code != want {
		t.Fatalf("%s %s: %d want %d: %s", method, path, code, want, body)
	}
	return body
}
func provisionInput(name, email string) map[string]any {
	return map[string]any{"business": map[string]any{"name": name}, "firstOutlet": map[string]any{"name": name}, "firstAdmin": map[string]any{"email": email, "fullName": "Owner " + name, "password": "owner-password-123"}}
}
func (p *platformTenantFixture) provision(t *testing.T, name, email string) provisionedTenant {
	t.Helper()
	body := p.expect(t, http.MethodPost, "/platform/businesses", provisionInput(name, email), p.token, 201, "")
	var tenant provisionedTenant
	if err := json.Unmarshal(body, &tenant); err != nil || tenant.Business.ID == 0 || tenant.Admin.Role != "ADMIN" {
		t.Fatalf("provision: %s err=%v", body, err)
	}
	if tenant.Outlet.Code != "001" {
		t.Fatalf("platform onboarding first outlet code=%q, want 001", tenant.Outlet.Code)
	}
	if strings.Contains(string(body), "owner-password-123") || strings.Contains(string(body), "password_hash") {
		t.Fatal("onboarding response exposed credentials")
	}
	p.businessIDs = append(p.businessIDs, tenant.Business.ID)
	tenant.email = email
	body = p.expect(t, http.MethodPost, "/auth/login", map[string]any{"email": email, "password": "owner-password-123"}, "", 200, "")
	var login sessionPayload
	if err := json.Unmarshal(body, &login); err != nil {
		t.Fatal(err)
	}
	tenant.token = login.Tokens.AccessToken
	body = p.expect(t, http.MethodPost, "/perfumes", map[string]any{"name": "Fresh", "description": "Original"}, tenant.token, 201, "")
	var perfume struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &perfume); err != nil || perfume.ID < 1 {
		t.Fatalf("perfume: %s", body)
	}
	tenant.perfume = perfume.ID
	return tenant
}
func (p *platformTenantFixture) supportRequest(t *testing.T, tenant provisionedTenant, scope string) int64 {
	t.Helper()
	body := p.expect(t, http.MethodPost, "/support-requests", map[string]any{"reason": "Owner requested configuration help", "accessScope": scope, "expiresAt": time.Now().Add(30 * time.Minute).UTC(), "confirmationPassword": "owner-password-123"}, tenant.token, 201, "")
	return decodePlatformID(t, body)
}
func decodePlatformID(t *testing.T, body []byte) int64 {
	t.Helper()
	var r struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &r); err != nil || r.ID < 1 {
		t.Fatalf("missing ID: %s", body)
	}
	return r.ID
}
func (p *platformTenantFixture) start(t *testing.T, tenant provisionedTenant, request int64) int64 {
	t.Helper()
	body := p.expect(t, http.MethodPost, fmt.Sprintf("/platform/businesses/%d/support-sessions", tenant.Business.ID), map[string]any{"supportRequestId": request}, p.token, 201, "")
	return decodePlatformID(t, body)
}
func supportPath(business, session int64, suffix string) string {
	return fmt.Sprintf("/platform/businesses/%d/support-sessions/%d/%s", business, session, suffix)
}

func TestPlatformTenantProvisioningIsolationAndSupport(t *testing.T) {
	p := newPlatformTenantFixture(t)
	a := p.provision(t, "Golden Laundry 19", "golden-"+p.suffix+"@example.test")
	b := p.provision(t, "Laundry Matahari", "matahari-"+p.suffix+"@example.test")
	for _, pair := range [][2]provisionedTenant{{a, b}, {b, a}} {
		own, other := pair[0], pair[1]
		p.expect(t, "GET", fmt.Sprintf("/outlets/%d", own.Outlet.ID), nil, own.token, 200, "")
		p.expect(t, "GET", fmt.Sprintf("/outlets/%d", other.Outlet.ID), nil, own.token, 404, "")
		p.expect(t, "PUT", fmt.Sprintf("/outlets/%d", other.Outlet.ID), map[string]any{"name": "Hijacked", "isActive": true}, own.token, 404, "")
		p.expect(t, "GET", fmt.Sprintf("/perfumes/%d", other.perfume), nil, own.token, 404, "")
		p.expect(t, "GET", "/platform/businesses", nil, own.token, 401, "")
	}
	p.expect(t, "GET", "/outlets", nil, p.token, 401, "")
	t.Run("duplicate_and_role_override", func(t *testing.T) {
		p.expect(t, "POST", "/platform/businesses", provisionInput("Duplicate", strings.ToUpper(a.email)), p.token, 409, "")
		in := provisionInput("Role override", "override-"+p.suffix+"@example.test")
		in["firstAdmin"].(map[string]any)["role"] = "PLATFORM_ADMIN"
		p.expect(t, "POST", "/platform/businesses", in, p.token, 400, "")
		delete(in["firstAdmin"].(map[string]any), "role")
		in["businessId"] = b.Business.ID
		p.expect(t, "POST", "/platform/businesses", in, p.token, 400, "")
	})
	t.Run("scope_and_target_isolation", func(t *testing.T) {
		request := p.supportRequest(t, a, "READ_ONLY")
		p.expect(t, "POST", fmt.Sprintf("/platform/businesses/%d/support-sessions", b.Business.ID), map[string]any{"supportRequestId": request}, p.token, 403, "")
		session := p.start(t, a, request)
		p.expect(t, "GET", supportPath(a.Business.ID, session, "diagnostics"), nil, p.token, 200, "")
		p.expect(t, "GET", supportPath(b.Business.ID, session, "diagnostics"), nil, p.token, 403, "")
		p.expect(t, "GET", supportPath(a.Business.ID, session, fmt.Sprintf("outlets/%d", b.Outlet.ID)), nil, p.token, 404, "")
		p.expect(t, "GET", supportPath(a.Business.ID, session, fmt.Sprintf("perfumes/%d", b.perfume)), nil, p.token, 404, "")
		p.expect(t, "PATCH", supportPath(a.Business.ID, session, fmt.Sprintf("perfumes/%d/description", a.perfume)), map[string]any{"description": "Forbidden"}, p.token, 403, "\"1\"")
		p.expect(t, "POST", fmt.Sprintf("/support-requests/%d/revoke", request), map[string]any{"reason": "Wrong tenant"}, b.token, 404, "")
		p.expect(t, "POST", fmt.Sprintf("/support-requests/%d/revoke", request), map[string]any{"reason": "Help complete"}, a.token, 200, "")
		p.expect(t, "GET", supportPath(a.Business.ID, session, "diagnostics"), nil, p.token, 403, "")
	})
	t.Run("write_and_owner_end", func(t *testing.T) {
		session := p.start(t, b, p.supportRequest(t, b, "READ_WRITE"))
		p.expect(t, "GET", supportPath(b.Business.ID, session, fmt.Sprintf("outlets/%d", a.Outlet.ID)), nil, p.token, 404, "")
		p.expect(t, "PATCH", supportPath(b.Business.ID, session, fmt.Sprintf("perfumes/%d/description", a.perfume)), map[string]any{"description": "Foreign mutation"}, p.token, 404, "\"1\"")

		path := supportPath(b.Business.ID, session, fmt.Sprintf("perfumes/%d/description", b.perfume))
		p.expect(t, "PATCH", path, map[string]any{"description": "Private description must be redacted"}, p.token, 200, "\"1\"")
		p.expect(t, "PATCH", path, map[string]any{"description": "Stale"}, p.token, 409, "\"1\"")
		p.expect(t, "POST", fmt.Sprintf("/support-sessions/%d/end", session), map[string]any{"reason": "Owner ended support"}, b.token, 200, "")
		p.expect(t, "GET", supportPath(b.Business.ID, session, "diagnostics"), nil, p.token, 403, "")
	})
	t.Run("expired", func(t *testing.T) {
		session := p.start(t, a, p.supportRequest(t, a, "READ_ONLY"))
		if _, err := p.f.adminPool.Exec(context.Background(), "UPDATE platform_support_sessions SET started_at=clock_timestamp()-interval '2 minutes',expires_at=clock_timestamp()-interval '1 minute' WHERE id=$1", session); err != nil {
			t.Fatal(err)
		}
		p.expect(t, "GET", supportPath(a.Business.ID, session, "diagnostics"), nil, p.token, 403, "")
	})
	t.Run("owner_confirmation", func(t *testing.T) {
		p.expect(t, "POST", "/support-requests", map[string]any{"reason": "Help", "accessScope": "READ_WRITE", "expiresAt": time.Now().Add(time.Minute), "confirmationPassword": "wrong-password"}, a.token, 403, "")
		p.expect(t, "POST", "/support-requests", map[string]any{"reason": "Help", "accessScope": "ADMIN", "expiresAt": time.Now().Add(time.Minute), "confirmationPassword": "owner-password-123"}, a.token, 400, "")
	})
	t.Run("concurrent_start", func(t *testing.T) {
		id := p.supportRequest(t, a, "READ_ONLY")
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				code, _ := p.request("POST", fmt.Sprintf("/platform/businesses/%d/support-sessions", a.Business.ID), map[string]any{"supportRequestId": id}, p.token, "")
				codes <- code
			}()
		}
		wg.Wait()
		close(codes)
		counts := map[int]int{}
		for code := range codes {
			counts[code]++
		}
		if counts[201] != 1 || counts[409] != 1 {
			t.Fatalf("start race: %v", counts)
		}
	})
	t.Run("concurrent_same_email", func(t *testing.T) {
		email := "race-" + p.suffix + "@example.test"
		type result struct {
			code int
			body []byte
		}
		results := make(chan result, 2)
		var wg sync.WaitGroup
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				code, body := p.request("POST", "/platform/businesses", provisionInput("Concurrent tenant", email), p.token, "")
				results <- result{code, body}
			}()
		}
		wg.Wait()
		close(results)
		counts := map[int]int{}
		for r := range results {
			counts[r.code]++
			if r.code == 201 {
				var tenant provisionedTenant
				if err := json.Unmarshal(r.body, &tenant); err != nil {
					t.Fatal(err)
				}
				p.businessIDs = append(p.businessIDs, tenant.Business.ID)
			}
		}
		if counts[201] != 1 || counts[409] != 1 {
			t.Fatalf("provision race: %v", counts)
		}
		var count int
		if err := p.f.pool.QueryRow(context.Background(), "SELECT count(*) FROM businesses WHERE name='Concurrent tenant'").Scan(&count); err != nil || count != 1 {
			t.Fatalf("partial concurrent provisioning: %d err=%v", count, err)
		}
	})
	t.Run("audit_views_and_secrets", func(t *testing.T) {
		for _, tenant := range []provisionedTenant{a, b} {
			body := p.expect(t, "GET", "/audit-logs", nil, tenant.token, 200, "")
			var page struct {
				Items []struct {
					BusinessID int64 `json:"business_id"`
				} `json:"items"`
			}
			if err := json.Unmarshal(body, &page); err != nil {
				t.Fatal(err)
			}
			if len(page.Items) == 0 {
				t.Fatal("missing business audit")
			}
			for _, item := range page.Items {
				if item.BusinessID != tenant.Business.ID {
					t.Fatal("business audit leaked another tenant")
				}
			}
		}
		staffToken := p.f.login(t).Tokens.AccessToken
		p.expect(t, "GET", "/audit-logs", nil, staffToken, 403, "")

		body := p.expect(t, "GET", "/support-audit", nil, a.token, 200, "")
		if strings.Contains(string(body), fmt.Sprintf("\"business_id\":%d,", b.Business.ID)) {
			t.Fatal("owner audit leaked foreign tenant")
		}
		p.expect(t, "GET", "/platform/audit-logs", nil, p.token, 200, "")
		var invalid int
		err := p.f.pool.QueryRow(context.Background(), `SELECT count(*) FROM platform_audit_logs WHERE business_id=ANY($1) AND (metadata::text ILIKE '%password%' OR metadata::text ILIKE '%Private description%' OR request_id IS NULL OR (support_session_id IS NOT NULL AND (reason IS NULL OR actor_platform_admin_id IS NULL) AND action NOT IN ('SUPPORT_ENDED')))`, p.businessIDs).Scan(&invalid)
		if err != nil || invalid != 0 {
			t.Fatalf("unsafe or incomplete audit: %d err=%v", invalid, err)
		}
	})
}

func TestPlatformTenantProvisioningAndSupportAuditRollback(t *testing.T) {
	p := newPlatformTenantFixture(t)
	ctx := context.Background()
	for _, table := range []string{"businesses", "outlets", "users", "user_outlets", "platform_audit_logs", "audit_logs"} {
		t.Run(table, func(t *testing.T) {
			name := "issue018_injected_failure"
			if _, err := p.f.adminPool.Exec(ctx, "CREATE FUNCTION "+name+"() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected test failure'; END $$"); err != nil {
				t.Fatal(err)
			}
			if _, err := p.f.adminPool.Exec(ctx, "CREATE TRIGGER "+name+" BEFORE INSERT ON "+table+" FOR EACH ROW EXECUTE FUNCTION "+name+"()"); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := p.f.adminPool.Exec(ctx, "DROP FUNCTION "+name+"() CASCADE"); err != nil {
					t.Error(err)
				}
			}()
			before := platformCounts(t, p)
			body := p.expect(t, "POST", "/platform/businesses", provisionInput("Rollback "+table, "rollback-"+table+"-"+p.suffix+"@example.test"), p.token, 500, "")
			if strings.Contains(string(body), "injected") {
				t.Fatal("SQL error leaked")
			}
			if after := platformCounts(t, p); before != after {
				t.Fatalf("partial provision %s: %v -> %v", table, before, after)
			}
		})
	}
	a := p.provision(t, "Golden Laundry 19", "audit-"+p.suffix+"@example.test")
	session := p.start(t, a, p.supportRequest(t, a, "READ_WRITE"))
	for _, table := range []string{"platform_audit_logs", "audit_logs"} {
		t.Run("support_"+table, func(t *testing.T) {
			pending := p.supportRequest(t, a, "READ_ONLY")
			if _, err := p.f.adminPool.Exec(ctx, "CREATE FUNCTION issue018_audit_failure() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'injected audit failure'; END $$"); err != nil {
				t.Fatal(err)
			}
			if _, err := p.f.adminPool.Exec(ctx, "CREATE TRIGGER issue018_audit_failure BEFORE INSERT ON "+table+" FOR EACH ROW EXECUTE FUNCTION issue018_audit_failure()"); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := p.f.adminPool.Exec(ctx, "DROP FUNCTION issue018_audit_failure() CASCADE"); err != nil {
					t.Error(err)
				}
			}()
			before := platformCounts(t, p)
			p.expect(t, "POST", fmt.Sprintf("/platform/businesses/%d/support-sessions", a.Business.ID), map[string]any{"supportRequestId": pending}, p.token, 500, "")
			var sessions int
			if err := p.f.pool.QueryRow(ctx, "SELECT count(*) FROM platform_support_sessions WHERE support_request_id=$1", pending).Scan(&sessions); err != nil || sessions != 0 {
				t.Fatalf("failed audit committed support session: %d err=%v", sessions, err)
			}
			p.expect(t, "GET", supportPath(a.Business.ID, session, "diagnostics"), nil, p.token, 500, "")
			p.expect(t, "PATCH", supportPath(a.Business.ID, session, fmt.Sprintf("perfumes/%d/description", a.perfume)), map[string]any{"description": "Must not persist"}, p.token, 500, "\"1\"")
			if after := platformCounts(t, p); before != after {
				t.Fatalf("partial audit %v -> %v", before, after)
			}
			var desc string
			var version int
			if err := p.f.pool.QueryRow(ctx, "SELECT description,version FROM perfumes WHERE id=$1", a.perfume).Scan(&desc, &version); err != nil || desc != "Original" || version != 1 {
				t.Fatalf("mutation committed on failed audit: %s v=%d err=%v", desc, version, err)
			}
		})
	}
}
func platformCounts(t *testing.T, p *platformTenantFixture) [6]int {
	t.Helper()
	var result [6]int
	for i, table := range []string{"businesses", "outlets", "users", "user_outlets", "platform_audit_logs", "audit_logs"} {
		if err := p.f.pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table).Scan(&result[i]); err != nil {
			t.Fatal(err)
		}
	}
	return result
}

func TestPlatformSupportConcurrentWriteAndImmediateRevocation(t *testing.T) {
	p := newPlatformTenantFixture(t)
	a := p.provision(t, "Laundry Matahari", "revoke-"+p.suffix+"@example.test")
	session := p.start(t, a, p.supportRequest(t, a, "READ_WRITE"))
	path := supportPath(a.Business.ID, session, fmt.Sprintf("perfumes/%d/description", a.perfume))
	t.Run("optimistic_write_race", func(t *testing.T) {
		var wg sync.WaitGroup
		codes := make(chan int, 2)
		for i := 0; i < 2; i++ {
			wg.Add(1)
			go func() {
				defer wg.Done()
				code, _ := p.request("PATCH", path, map[string]any{"description": "Concurrent edit"}, p.token, "\"1\"")
				codes <- code
			}()
		}
		wg.Wait()
		close(codes)
		counts := map[int]int{}
		for code := range codes {
			counts[code]++
		}
		if counts[200] != 1 || counts[409] != 1 {
			t.Fatalf("write race: %v", counts)
		}
	})
	t.Run("end_serializes_with_inflight_write", func(t *testing.T) {
		ctx := context.Background()
		blocker, err := p.f.adminPool.Begin(ctx)
		if err != nil {
			t.Fatal(err)
		}
		defer blocker.Rollback(ctx)
		if _, err := blocker.Exec(ctx, "SELECT id FROM perfumes WHERE id=$1 FOR UPDATE", a.perfume); err != nil {
			t.Fatal(err)
		}
		writes := make(chan int, 1)
		go func() {
			code, _ := p.request("PATCH", path, map[string]any{"description": "In flight"}, p.token, "\"2\"")
			writes <- code
		}()
		deadline := time.Now().Add(5 * time.Second)
		blocked := false
		for time.Now().Before(deadline) {
			var count int
			if err := p.f.adminPool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE usename='launlog_runtime' AND query LIKE '%SupportUpdatePerfumeDescription%' AND cardinality(pg_blocking_pids(pid))>0`).Scan(&count); err != nil {
				t.Fatal(err)
			}
			if count > 0 {
				blocked = true
				break
			}
			time.Sleep(10 * time.Millisecond)
		}
		if !blocked {
			t.Fatal("support write did not acquire grant before blocking on record")
		}
		ends := make(chan int, 1)
		go func() {
			code, _ := p.request("POST", fmt.Sprintf("/platform/businesses/%d/support-sessions/%d/end", a.Business.ID, session), map[string]any{"reason": "Support complete"}, p.token, "")
			ends <- code
		}()
		select {
		case code := <-ends:
			t.Fatalf("end returned before earlier write completed: %d", code)
		case <-time.After(100 * time.Millisecond):
		}
		if err := blocker.Commit(ctx); err != nil {
			t.Fatal(err)
		}
		select {
		case code := <-writes:
			if code != 200 {
				t.Fatalf("inflight write=%d", code)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("write blocked indefinitely")
		}
		select {
		case code := <-ends:
			if code != 200 {
				t.Fatalf("end=%d", code)
			}
		case <-time.After(5 * time.Second):
			t.Fatal("revocation blocked indefinitely")
		}
		p.expect(t, "PATCH", path, map[string]any{"description": "After revocation"}, p.token, 403, "\"3\"")
		p.expect(t, "GET", supportPath(a.Business.ID, session, "diagnostics"), nil, p.token, 403, "")
	})
}

func TestPlatformTenantDeactivationAndAuditPrivileges(t *testing.T) {
	p := newPlatformTenantFixture(t)
	a := p.provision(t, "Golden Laundry 19", "active-"+p.suffix+"@example.test")
	session := p.start(t, a, p.supportRequest(t, a, "READ_ONLY"))
	activation := fmt.Sprintf("/platform/businesses/%d/activation", a.Business.ID)
	p.expect(t, "PATCH", activation, map[string]any{"isActive": false, "reason": "Owner requested suspension"}, p.token, 200, "")
	p.expect(t, "GET", "/outlets", nil, a.token, 401, "")
	p.expect(t, "GET", supportPath(a.Business.ID, session, "diagnostics"), nil, p.token, 403, "")
	p.expect(t, "POST", "/auth/login", map[string]any{"email": a.email, "password": "owner-password-123"}, "", 401, "")
	p.expect(t, "PATCH", activation, map[string]any{"isActive": true, "reason": "Owner resumed business"}, p.token, 200, "")
	p.expect(t, "GET", "/outlets", nil, a.token, 401, "")
	p.expect(t, "GET", supportPath(a.Business.ID, session, "diagnostics"), nil, p.token, 403, "")
	p.expect(t, "POST", "/auth/login", map[string]any{"email": a.email, "password": "owner-password-123"}, "", 200, "")
	ctx := context.Background()
	if err := p.f.database.VerifyLeastPrivilege(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := p.f.adminPool.Exec(ctx, "GRANT TRUNCATE ON audit_logs,platform_audit_logs TO launlog_runtime"); err != nil {
		t.Fatal(err)
	}
	defer func() {
		if _, err := p.f.adminPool.Exec(ctx, "REVOKE TRUNCATE ON audit_logs,platform_audit_logs FROM launlog_runtime"); err != nil {
			t.Error(err)
		}
	}()
	if err := p.f.database.VerifyLeastPrivilege(ctx); err == nil {
		t.Fatal("TRUNCATE privilege passed runtime verification")
	}
	before := platformCounts(t, p)
	if _, err := p.f.pool.Exec(ctx, "TRUNCATE audit_logs,platform_audit_logs"); err == nil {
		t.Fatal("audit truncate accepted")
	}
	if after := platformCounts(t, p); before != after {
		t.Fatal("audit rows lost")
	}
}

func TestPlatformSupportExpiryDuringMutationRollsBack(t *testing.T) {
	p := newPlatformTenantFixture(t)
	a := p.provision(t, "Laundry Matahari", "expiry-"+p.suffix+"@example.test")
	session := p.start(t, a, p.supportRequest(t, a, "READ_WRITE"))
	ctx := context.Background()
	if _, err := p.f.adminPool.Exec(ctx, "UPDATE platform_support_sessions SET expires_at=clock_timestamp()+interval '2 seconds' WHERE id=$1", session); err != nil {
		t.Fatal(err)
	}
	blocker, err := p.f.adminPool.Begin(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer blocker.Rollback(ctx)
	if _, err := blocker.Exec(ctx, "SELECT id FROM perfumes WHERE id=$1 FOR UPDATE", a.perfume); err != nil {
		t.Fatal(err)
	}
	done := make(chan int, 1)
	go func() {
		code, _ := p.request("PATCH", supportPath(a.Business.ID, session, fmt.Sprintf("perfumes/%d/description", a.perfume)), map[string]any{"description": "Too late"}, p.token, "\"1\"")
		done <- code
	}()
	deadline := time.Now().Add(time.Second)
	blocked := false
	for time.Now().Before(deadline) {
		var n int
		if err := p.f.adminPool.QueryRow(ctx, `SELECT count(*) FROM pg_stat_activity WHERE usename='launlog_runtime' AND query LIKE '%SupportUpdatePerfumeDescription%' AND cardinality(pg_blocking_pids(pid))>0`).Scan(&n); err != nil {
			t.Fatal(err)
		}
		if n > 0 {
			blocked = true
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	if !blocked {
		t.Fatal("mutation did not reach record lock before expiry")
	}
	time.Sleep(2100 * time.Millisecond)
	if err := blocker.Commit(ctx); err != nil {
		t.Fatal(err)
	}
	select {
	case code := <-done:
		if code != 403 {
			t.Fatalf("expired in-flight mutation=%d", code)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("mutation did not finish")
	}
	var desc string
	var version int
	if err := p.f.pool.QueryRow(ctx, "SELECT description,version FROM perfumes WHERE id=$1", a.perfume).Scan(&desc, &version); err != nil || desc != "Original" || version != 1 {
		t.Fatalf("expired mutation persisted: %s v=%d err=%v", desc, version, err)
	}
}
