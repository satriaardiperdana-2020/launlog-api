package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/labstack/echo/v4"
)

type healthCheckerStub struct {
	err error
}

func (s healthCheckerStub) Ping(context.Context) error {
	return s.err
}

func TestHealthAvailable(t *testing.T) {
	e := echo.New()
	handler := NewHealthHandler(healthCheckerStub{})
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()

	if err := handler.Health(e.NewContext(request, recorder)); err != nil {
		t.Fatalf("Health() error = %v", err)
	}

	if recorder.Code != http.StatusOK {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusOK)
	}
	if recorder.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Errorf("body = %q, want healthy response", recorder.Body.String())
	}
}

func TestHealthUnavailable(t *testing.T) {
	e := echo.New()
	handler := NewHealthHandler(healthCheckerStub{err: errors.New("database unavailable")})
	request := httptest.NewRequest(http.MethodGet, "/health", nil)
	recorder := httptest.NewRecorder()

	if err := handler.Health(e.NewContext(request, recorder)); err != nil {
		t.Fatalf("Health() error = %v", err)
	}

	if recorder.Code != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want %d", recorder.Code, http.StatusServiceUnavailable)
	}
	if recorder.Body.String() != "{\"status\":\"unavailable\"}\n" {
		t.Errorf("body = %q, want unavailable response", recorder.Body.String())
	}
}
