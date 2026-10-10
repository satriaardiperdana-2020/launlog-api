package server

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/satriaardiperdana-2020/launlog-api/internal/config"
)

func TestDevelopmentSwaggerUIAndContract(t *testing.T) {
	e := New(config.Config{Environment: "development"}, nil, nil)

	tests := []struct {
		path        string
		status      int
		contentType string
		contains    string
	}{
		{path: "/swagger", status: http.StatusTemporaryRedirect},
		{path: "/swagger/", status: http.StatusOK, contentType: "text/html", contains: "/swagger/swagger-init.js"},
		{path: "/swagger/swagger-init.js", status: http.StatusOK, contentType: "application/javascript", contains: `url: "/openapi.yaml"`},
		{path: "/swagger/swagger-ui.css", status: http.StatusOK, contentType: "text/css"},
		{path: "/swagger/swagger-ui-bundle.js", status: http.StatusOK, contentType: "javascript"},
		{path: "/openapi.yaml", status: http.StatusOK, contentType: "application/yaml", contains: "openapi: 3.0.3"},
	}

	for _, test := range tests {
		t.Run(test.path, func(t *testing.T) {
			recorder := httptest.NewRecorder()
			e.ServeHTTP(recorder, httptest.NewRequest(http.MethodGet, test.path, nil))
			if recorder.Code != test.status {
				t.Fatalf("status = %d, want %d; body: %s", recorder.Code, test.status, recorder.Body.String())
			}
			if test.contentType != "" && !strings.Contains(recorder.Header().Get("Content-Type"), test.contentType) {
				t.Errorf("Content-Type = %q, want it to contain %q", recorder.Header().Get("Content-Type"), test.contentType)
			}
			if test.contains != "" && !strings.Contains(recorder.Body.String(), test.contains) {
				t.Errorf("response body does not contain %q: %.180s", test.contains, recorder.Body.String())
			}
		})
	}
}

func TestSwaggerRoutesAreDisabledOutsideDevelopment(t *testing.T) {
	e := New(config.Config{Environment: "production"}, nil, nil)

	for _, route := range e.Routes() {
		if route.Path == "/swagger" || route.Path == "/swagger/" || route.Path == "/swagger/*" || route.Path == "/swagger/swagger-init.js" || route.Path == "/openapi.yaml" {
			t.Errorf("documentation route %q should not be registered outside development", route.Path)
		}
	}
}

func TestOwnerRegistrationRouteIsClosedByDefault(t *testing.T) {
	e := New(config.Config{Environment: "test"}, nil, nil)
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{"email":"owner@example.test"}`))
	request.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusForbidden || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("disabled registration response = %d, Cache-Control=%q, body=%s", recorder.Code, recorder.Header().Get("Cache-Control"), recorder.Body.String())
	}
}

func TestOwnerRegistrationRejectsClientAuthorityAndLimitsRequests(t *testing.T) {
	for _, body := range []string{
		`{"email":"x@example.test","password":"password-123","fullName":"X","business":{"name":"B"},"firstOutlet":{"code":"M","name":"M"},"role":"PLATFORM_ADMIN"}`,
		`{"email":"x@example.test","password":"password-123","fullName":"X","business":{"name":"B","business_id":1},"firstOutlet":{"code":"M","name":"M"}}`,
		`{"email":"x@example.test","password":"password-123","fullName":"X","business":{"name":"B"},"firstOutlet":{"code":"M","name":"M"},"permissions":["ADMIN"]}`,
	} {
		e := New(config.Config{Environment: "test", OwnerRegistrationEnabled: true}, nil, nil)
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(body))
		request.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("client supplied authority accepted: status=%d body=%s", recorder.Code, recorder.Body.String())
		}
	}

	e := New(config.Config{Environment: "test", OwnerRegistrationEnabled: true}, nil, nil)
	for i := 0; i < 3; i++ {
		recorder := httptest.NewRecorder()
		request := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{}`))
		request.Header.Set("Content-Type", "application/json")
		e.ServeHTTP(recorder, request)
		if recorder.Code != http.StatusBadRequest {
			t.Fatalf("invalid registration status=%d", recorder.Code)
		}
	}
	recorder := httptest.NewRecorder()
	request := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{}`))
	request.Header.Set("Content-Type", "application/json")
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusTooManyRequests || recorder.Header().Get("Retry-After") == "" || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("registration rate limit status=%d retry-after=%q cache-control=%q", recorder.Code, recorder.Header().Get("Retry-After"), recorder.Header().Get("Cache-Control"))
	}
}

func TestOwnerRegistrationBodyLimitAndContentType(t *testing.T) {
	e := New(config.Config{Environment: "test", OwnerRegistrationEnabled: true}, nil, nil)
	large := strings.Repeat("x", 16*1024+1)
	request := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(large))
	request.Header.Set("Content-Type", "application/json")
	recorder := httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusRequestEntityTooLarge || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("oversized registration status=%d cache-control=%q", recorder.Code, recorder.Header().Get("Cache-Control"))
	}

	streamed := httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{}`+strings.Repeat(" ", 16*1024)))
	streamed.ContentLength = -1
	streamed.Header.Set("Content-Type", "application/json")
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, streamed)
	if recorder.Code != http.StatusRequestEntityTooLarge || recorder.Header().Get("Cache-Control") != "no-store" {
		t.Fatalf("streamed oversized registration status=%d cache-control=%q body=%s", recorder.Code, recorder.Header().Get("Cache-Control"), recorder.Body.String())
	}
	request = httptest.NewRequest(http.MethodPost, "/auth/register", strings.NewReader(`{}`))
	recorder = httptest.NewRecorder()
	e.ServeHTTP(recorder, request)
	if recorder.Code != http.StatusUnsupportedMediaType {
		t.Fatalf("missing JSON Content-Type status=%d", recorder.Code)
	}
}
