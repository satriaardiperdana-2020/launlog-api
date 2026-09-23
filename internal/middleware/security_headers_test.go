package middleware

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestSecurityHeadersAndNoStore(t *testing.T) {
	e := echo.New()
	e.GET("/auth/me", func(c echo.Context) error { return c.NoContent(http.StatusOK) }, SecurityHeaders, NoStore)
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, "/auth/me", nil))
	for name, want := range map[string]string{
		echo.HeaderXContentTypeOptions:   "nosniff",
		echo.HeaderXFrameOptions:         "DENY",
		echo.HeaderReferrerPolicy:        "no-referrer",
		echo.HeaderContentSecurityPolicy: "default-src 'none'; frame-ancestors 'none'; base-uri 'none'",
		echo.HeaderCacheControl:          "no-store",
		"Pragma":                         "no-cache",
		"Cross-Origin-Resource-Policy":   "same-origin",
	} {
		if got := recorder.Header().Get(name); got != want {
			t.Errorf("%s=%q, want %q", name, got, want)
		}
	}
}

func TestCORSUsesOnlyExplicitOrigins(t *testing.T) {
	e := echo.New()
	e.Use(CORSConfig([]string{"https://app.example"}))
	e.GET("/resource", func(c echo.Context) error { return c.NoContent(http.StatusOK) })
	call := func(origin string) *httptest.ResponseRecorder {
		req := httptest.NewRequest(http.MethodOptions, "/resource", nil)
		req.Header.Set(echo.HeaderOrigin, origin)
		req.Header.Set(echo.HeaderAccessControlRequestMethod, http.MethodGet)
		recorder := httptest.NewRecorder()
		e.ServeHTTP(recorder, req)
		return recorder
	}
	allowed := call("https://app.example")
	if got := allowed.Header().Get(echo.HeaderAccessControlAllowOrigin); got != "https://app.example" {
		t.Fatalf("allowed ACAO=%q", got)
	}
	denied := call("https://attacker.example")
	if got := denied.Header().Get(echo.HeaderAccessControlAllowOrigin); got != "" {
		t.Fatalf("unlisted origin received ACAO=%q", got)
	}
}
