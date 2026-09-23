package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestAuthLimiterIsBoundedAndIgnoresForwardedIP(t *testing.T) {
	e := echo.New()
	e.IPExtractor = echo.ExtractIPDirect()
	limiter := NewAuthLimiter(1, 1, time.Minute)
	e.POST("/auth/login", func(c echo.Context) error { return c.NoContent(http.StatusOK) }, limiter.Middleware)
	call := func(remote, forwarded string) int {
		req := httptest.NewRequest(http.MethodPost, "/auth/login", nil)
		req.RemoteAddr = remote
		req.Header.Set("X-Forwarded-For", forwarded)
		rec := httptest.NewRecorder()
		e.ServeHTTP(rec, req)
		return rec.Code
	}
	if got := call("192.0.2.1:1234", "198.51.100.1"); got != 200 {
		t.Fatal(got)
	}
	if got := call("192.0.2.1:1234", "198.51.100.2"); got != 429 {
		t.Fatalf("spoofed header bypassed limiter: %d", got)
	}
	if got := call("192.0.2.2:1234", "198.51.100.3"); got != 429 {
		t.Fatalf("full limiter failed open: %d", got)
	}
}
