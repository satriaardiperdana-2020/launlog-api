package handlers

import (
	"context"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
)

// HealthChecker is the database behavior required by the health endpoint.
type HealthChecker interface {
	Ping(context.Context) error
}

// HealthHandler serves application health information.
type HealthHandler struct {
	database         HealthChecker
	readinessTimeout time.Duration
}

// NewHealthHandler creates liveness and readiness handlers. Readiness checks
// use a bounded context so an unavailable database cannot hold health probes.
func NewHealthHandler(database HealthChecker, readinessTimeout time.Duration) *HealthHandler {
	return &HealthHandler{database: database, readinessTimeout: readinessTimeout}
}

// Live reports that the HTTP process is running. It deliberately does not
// query dependencies, so orchestrators do not restart a healthy process while
// PostgreSQL is temporarily unavailable.
func (h *HealthHandler) Live(c echo.Context) error {
	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Ready reports whether the API can serve requests that depend on PostgreSQL.
func (h *HealthHandler) Ready(c echo.Context) error {
	ctx, cancel := context.WithTimeout(c.Request().Context(), h.readinessTimeout)
	defer cancel()

	if err := h.database.Ping(ctx); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}

// Health remains a compatibility alias for readiness.
func (h *HealthHandler) Health(c echo.Context) error { return h.Ready(c) }
