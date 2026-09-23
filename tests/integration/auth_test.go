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
	"reflect"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/labstack/echo/v4"
	"golang.org/x/crypto/bcrypt"

	"github.com/satriaardiperdana-2020/launlog-api/internal/handlers"
	authmiddleware "github.com/satriaardiperdana-2020/launlog-api/internal/middleware"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
)

type authFixture struct {
	pool                         *pgxpool.Pool
	database                     *repository.Postgres
	echo                         *echo.Echo
	businessID, outletID, userID int64
}

func newAuthFixture(t *testing.T) *authFixture {
	t.Helper()
	url := os.Getenv("TEST_DATABASE_URL")
	if url == "" {
		t.Skip("TEST_DATABASE_URL is required")
	}
	ctx := context.Background()
	pool, err := pgxpool.New(ctx, url)
	if err != nil {
		t.Fatal(err)
	}
	database, err := repository.NewPostgres(ctx, url, "Asia/Jakarta", 5*time.Second, 10, 0)
	if err != nil {
		pool.Close()
		t.Fatal(err)
	}
	f := &authFixture{pool: pool, database: database, echo: echo.New()}
	f.echo.IPExtractor = echo.ExtractIPDirect()
	t.Cleanup(func() {
		// Delete only records created by this test, in foreign-key order.
		_, _ = pool.Exec(ctx, "DELETE FROM refresh_tokens WHERE business_id=$1", f.businessID)
		_, _ = pool.Exec(ctx, "DELETE FROM session_families WHERE business_id=$1", f.businessID)
		_, _ = pool.Exec(ctx, "DELETE FROM user_outlets WHERE business_id=$1", f.businessID)
		_, _ = pool.Exec(ctx, "DELETE FROM users WHERE business_id=$1", f.businessID)
		_, _ = pool.Exec(ctx, "DELETE FROM outlets WHERE business_id=$1", f.businessID)
		_, _ = pool.Exec(ctx, "DELETE FROM businesses WHERE id=$1", f.businessID)
		database.Close()
		pool.Close()
	})
	if err := pool.QueryRow(ctx, "INSERT INTO businesses (name) VALUES ($1) RETURNING id", fmt.Sprintf("Auth Integration %d", time.Now().UnixNano())).Scan(&f.businessID); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(ctx, "INSERT INTO outlets (business_id, code, name) VALUES ($1,'AUTH','Auth outlet') RETURNING id", f.businessID).Scan(&f.outletID); err != nil {
		t.Fatal(err)
	}
	hash, err := security.HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	email := fmt.Sprintf("auth-%d@example.test", time.Now().UnixNano())
	if err := pool.QueryRow(ctx, "INSERT INTO users (business_id,email,full_name,password_hash,role) VALUES ($1,$2,'Auth user',$3,'LAUNDRY_STAFF') RETURNING id", f.businessID, email, hash).Scan(&f.userID); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(ctx, "INSERT INTO user_outlets (business_id,user_id,outlet_id) VALUES ($1,$2,$3)", f.businessID, f.userID, f.outletID); err != nil {
		t.Fatal(err)
	}
	tokens, err := security.NewTokenManager("integration-test-signing-secret-at-least-32-bytes", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	handler := handlers.NewAuthHandler(database, tokens, time.Hour)
	limiter := authmiddleware.NewAuthLimiter(4096, 10, time.Minute)
	bodyLimit := authmiddleware.AuthBodyLimit(4096)
	f.echo.POST("/auth/login", handler.Login, bodyLimit, limiter.Middleware)
	f.echo.POST("/auth/refresh", handler.Refresh, bodyLimit, limiter.Middleware)
	f.echo.POST("/auth/logout", handler.Logout, bodyLimit, authmiddleware.AuthenticateForLogout(database, tokens))
	f.echo.GET("/auth/me", handler.Me, authmiddleware.Authenticate(database, tokens))
	return f
}

func (f *authFixture) request(path string, body any, bearer string) (int, []byte) {
	var encoded []byte
	if body != nil {
		encoded, _ = json.Marshal(body)
	}
	req := httptest.NewRequest(http.MethodPost, path, bytes.NewReader(encoded))
	if path == "/auth/me" {
		req.Method = http.MethodGet
	}
	req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	if bearer != "" {
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+bearer)
	}
	rec := httptest.NewRecorder()
	f.echo.ServeHTTP(rec, req)
	return rec.Code, rec.Body.Bytes()
}

type sessionPayload struct {
	Tokens struct {
		AccessToken           string    `json:"accessToken"`
		RefreshToken          string    `json:"refreshToken"`
		RefreshTokenExpiresAt time.Time `json:"refreshTokenExpiresAt"`
	} `json:"tokens"`
}

func (f *authFixture) login(t *testing.T) sessionPayload {
	t.Helper()
	var email string
	if err := f.pool.QueryRow(context.Background(), "SELECT email FROM users WHERE id=$1", f.userID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	code, body := f.request("/auth/login", map[string]string{"email": email, "password": "correct-password"}, "")
	if code != http.StatusOK {
		t.Fatalf("login status %d: %s", code, body)
	}
	var payload sessionPayload
	if err := json.Unmarshal(body, &payload); err != nil {
		t.Fatal(err)
	}
	if payload.Tokens.AccessToken == "" || payload.Tokens.RefreshToken == "" {
		t.Fatal("missing token pair")
	}
	var stored time.Time
	if err := f.pool.QueryRow(context.Background(), "SELECT expires_at FROM refresh_tokens WHERE token_hash=$1", security.HashRefreshToken(payload.Tokens.RefreshToken)).Scan(&stored); err != nil {
		t.Fatal(err)
	}
	if !payload.Tokens.RefreshTokenExpiresAt.Equal(stored) {
		t.Fatal("response expiry differs from stored expiry")
	}
	return payload
}

func TestAuthReplayAndConcurrentRotation(t *testing.T) {
	f := newAuthFixture(t)
	initial := f.login(t)
	var wg sync.WaitGroup
	results := make([]struct {
		code int
		body []byte
	}, 2)
	for i := range results {
		wg.Add(1)
		go func(index int) {
			defer wg.Done()
			results[index].code, results[index].body = f.request("/auth/refresh", map[string]string{"refreshToken": initial.Tokens.RefreshToken}, "")
		}(i)
	}
	wg.Wait()
	codes := []int{results[0].code, results[1].code}
	if !reflect.DeepEqual(codes, []int{200, 401}) && !reflect.DeepEqual(codes, []int{401, 200}) {
		t.Fatalf("concurrent refresh statuses: %v", codes)
	}
	var child sessionPayload
	for _, result := range results {
		if result.code == 200 {
			if err := json.Unmarshal(result.body, &child); err != nil {
				t.Fatal(err)
			}
		}
	}
	if code, _ := f.request("/auth/refresh", map[string]string{"refreshToken": child.Tokens.RefreshToken}, ""); code != 401 {
		t.Fatalf("descendant of replayed token remained usable: %d", code)
	}
	if code, _ := f.request("/auth/me", nil, child.Tokens.AccessToken); code != 401 {
		t.Fatalf("access token survived replay revocation: %d", code)
	}
	var familyRevoked bool
	if err := f.pool.QueryRow(context.Background(), "SELECT sf.revoked_at IS NOT NULL FROM session_families sf JOIN refresh_tokens rt ON rt.family_id=sf.id WHERE rt.token_hash=$1", security.HashRefreshToken(initial.Tokens.RefreshToken)).Scan(&familyRevoked); err != nil || !familyRevoked {
		t.Fatalf("replay revocation was not committed: %v", err)
	}
}

func TestAuthRefreshLogoutRace(t *testing.T) {
	f := newAuthFixture(t)
	initial := f.login(t)
	var wg sync.WaitGroup
	var refreshCode, logoutCode int
	var refreshBody []byte
	wg.Add(2)
	go func() {
		defer wg.Done()
		refreshCode, refreshBody = f.request("/auth/refresh", map[string]string{"refreshToken": initial.Tokens.RefreshToken}, "")
	}()
	go func() {
		defer wg.Done()
		logoutCode, _ = f.request("/auth/logout", map[string]string{"refreshToken": initial.Tokens.RefreshToken}, initial.Tokens.AccessToken)
	}()
	wg.Wait()
	if logoutCode != 204 || (refreshCode != 200 && refreshCode != 401) {
		t.Fatalf("refresh/logout race: refresh=%d logout=%d", refreshCode, logoutCode)
	}
	if code, _ := f.request("/auth/me", nil, initial.Tokens.AccessToken); code != 401 {
		t.Fatalf("old access survived logout: %d", code)
	}
	if refreshCode == 200 {
		var child sessionPayload
		if err := json.Unmarshal(refreshBody, &child); err != nil {
			t.Fatal(err)
		}
		if code, _ := f.request("/auth/me", nil, child.Tokens.AccessToken); code != 401 {
			t.Fatalf("new access survived logout: %d", code)
		}
	}
}

func TestAuthInactiveBusinessAndOutlet(t *testing.T) {
	f := newAuthFixture(t)
	initial := f.login(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, "UPDATE outlets SET is_active=FALSE WHERE id=$1", f.outletID); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.request("/auth/me", nil, initial.Tokens.AccessToken); code != 401 {
		t.Fatalf("inactive outlet authorized: %d", code)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE businesses SET is_active=FALSE WHERE id=$1", f.businessID); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.request("/auth/refresh", map[string]string{"refreshToken": initial.Tokens.RefreshToken}, ""); code != 401 {
		t.Fatalf("inactive business refreshed: %d", code)
	}
}

func TestAuthInactiveUserAndCrossBusinessAuthority(t *testing.T) {
	first := newAuthFixture(t)
	second := newAuthFixture(t)
	initial := first.login(t)
	other := second.login(t)
	if code, _ := first.request("/auth/logout", map[string]string{"refreshToken": other.Tokens.RefreshToken}, initial.Tokens.AccessToken); code != 401 {
		t.Fatalf("cross-business logout authorized: %d", code)
	}
	manager, err := security.NewTokenManager("integration-test-signing-secret-at-least-32-bytes", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claims, err := manager.ParseAccessToken(initial.Tokens.AccessToken)
	if err != nil {
		t.Fatal(err)
	}
	crossToken, _, err := manager.NewAccessToken(first.userID, second.businessID, claims.SessionID, "LAUNDRY_STAFF", []int64{first.outletID}, time.Now())
	if err != nil {
		t.Fatal(err)
	}
	if code, _ := first.request("/auth/me", nil, crossToken); code != 401 {
		t.Fatalf("cross-business access authorized: %d", code)
	}
	if _, err := first.pool.Exec(context.Background(), "UPDATE users SET is_active=FALSE WHERE id=$1", first.userID); err != nil {
		t.Fatal(err)
	}
	if code, _ := first.request("/auth/me", nil, initial.Tokens.AccessToken); code != 401 {
		t.Fatalf("inactive user authorized: %d", code)
	}
}

func TestLegacyShortPasswordCanStillLogin(t *testing.T) {
	f := newAuthFixture(t)
	legacyHash, err := bcrypt.GenerateFromPassword([]byte("legacy"), 12)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(context.Background(), "UPDATE users SET password_hash=$1 WHERE id=$2", string(legacyHash), f.userID); err != nil {
		t.Fatal(err)
	}
	var email string
	if err := f.pool.QueryRow(context.Background(), "SELECT email FROM users WHERE id=$1", f.userID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	if code, _ := f.request("/auth/login", map[string]string{"email": email, "password": "legacy"}, ""); code != 200 {
		t.Fatalf("existing short password was rejected at login: %d", code)
	}
}

func TestAuthCredentialFailuresAndBodyLimit(t *testing.T) {
	f := newAuthFixture(t)
	var email string
	if err := f.pool.QueryRow(context.Background(), "SELECT email FROM users WHERE id=$1", f.userID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	wrongCode, wrongBody := f.request("/auth/login", map[string]string{"email": email, "password": "wrong-password"}, "")
	missingCode, missingBody := f.request("/auth/login", map[string]string{"email": "missing-user@example.test", "password": "wrong-password"}, "")
	if wrongCode != 401 || missingCode != 401 || !bytes.Equal(wrongBody, missingBody) {
		t.Fatalf("credential failures differ: wrong=%d missing=%d", wrongCode, missingCode)
	}
	if code, _ := f.request("/auth/login", map[string]string{"email": email, "password": string(bytes.Repeat([]byte("x"), 5000))}, ""); code != 413 {
		t.Fatalf("oversized login body accepted: %d", code)
	}
	largeJSON, _ := json.Marshal(map[string]string{"email": email, "password": string(bytes.Repeat([]byte("x"), 5000))})
	chunked := httptest.NewRequest(http.MethodPost, "/auth/login", bytes.NewReader(largeJSON))
	chunked.ContentLength = -1
	chunked.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
	recorder := httptest.NewRecorder()
	f.echo.ServeHTTP(recorder, chunked)
	if recorder.Code != 413 {
		t.Fatalf("oversized unknown-length body accepted: %d", recorder.Code)
	}
}
