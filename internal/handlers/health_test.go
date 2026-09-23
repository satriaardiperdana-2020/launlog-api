package handlers

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

type healthCheckerStub struct {
	err  error
	ping func(context.Context) error
}

func (s healthCheckerStub) Ping(ctx context.Context) error {
	if s.ping != nil {
		return s.ping(ctx)
	}
	return s.err
}

func TestHealthAvailable(t *testing.T) {
	e := echo.New()
	handler := NewHealthHandler(healthCheckerStub{}, time.Second)
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
	handler := NewHealthHandler(healthCheckerStub{err: errors.New("database unavailable")}, time.Second)
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

func TestLivenessDoesNotRequireDatabase(t *testing.T) {
	e := echo.New()
	handler := NewHealthHandler(healthCheckerStub{err: errors.New("database unavailable")}, time.Second)
	request := httptest.NewRequest(http.MethodGet, "/livez", nil)
	recorder := httptest.NewRecorder()

	if err := handler.Live(e.NewContext(request, recorder)); err != nil {
		t.Fatalf("Live() error = %v", err)
	}
	if recorder.Code != http.StatusOK || recorder.Body.String() != "{\"status\":\"ok\"}\n" {
		t.Fatalf("liveness response = (%d, %q), want healthy response", recorder.Code, recorder.Body.String())
	}
}

func TestReadinessUsesConfiguredTimeout(t *testing.T) {
	e := echo.New()
	called := false
	handler := NewHealthHandler(healthCheckerStub{ping: func(ctx context.Context) error {
		called = true
		if _, ok := ctx.Deadline(); !ok {
			t.Fatal("readiness database ping has no deadline")
		}
		return nil
	}}, time.Second)
	request := httptest.NewRequest(http.MethodGet, "/readyz", nil)
	recorder := httptest.NewRecorder()

	if err := handler.Ready(e.NewContext(request, recorder)); err != nil {
		t.Fatalf("Ready() error = %v", err)
	}
	if !called || recorder.Code != http.StatusOK {
		t.Fatalf("readiness call = (%t, %d), want (true, 200)", called, recorder.Code)
	}
}
