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
