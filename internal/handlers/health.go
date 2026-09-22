package handlers

import (
	"context"
	"net/http"

	"github.com/labstack/echo/v4"
)

// HealthChecker is the database behavior required by the health endpoint.
type HealthChecker interface {
	Ping(context.Context) error
}

// HealthHandler serves application health information.
type HealthHandler struct {
	database HealthChecker
}

// NewHealthHandler creates a health handler backed by a database check.
func NewHealthHandler(database HealthChecker) *HealthHandler {
	return &HealthHandler{database: database}
}

// Health reports whether the API and its database dependency are available.
func (h *HealthHandler) Health(c echo.Context) error {
	if err := h.database.Ping(c.Request().Context()); err != nil {
		return c.JSON(http.StatusServiceUnavailable, map[string]string{"status": "unavailable"})
	}

	return c.JSON(http.StatusOK, map[string]string{"status": "ok"})
}
