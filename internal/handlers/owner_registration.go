package handlers

import (
	"errors"
	"mime"
	"net/http"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
	"github.com/satriaardiperdana-2020/launlog-api/internal/service"
	"github.com/satriaardiperdana-2020/launlog-api/internal/timezone"
)

type OwnerRegistrationHandler struct {
	registration *service.OwnerRegistration
	enabled      bool
}

func NewOwnerRegistrationHandler(database *repository.Postgres, tokens *security.TokenManager, refreshTTL time.Duration, enabled bool) *OwnerRegistrationHandler {
	return &OwnerRegistrationHandler{registration: service.NewOwnerRegistration(database, tokens, refreshTTL), enabled: enabled}
}

func (h *OwnerRegistrationHandler) Register(c echo.Context) error {
	if !h.enabled {
		return c.JSON(http.StatusForbidden, map[string]string{"code": "REGISTRATION_DISABLED", "message": "Registration is currently unavailable."})
	}
	mediaType, _, err := mime.ParseMediaType(c.Request().Header.Get(echo.HeaderContentType))
	if err != nil || mediaType != echo.MIMEApplicationJSON {
		return c.JSON(http.StatusUnsupportedMediaType, map[string]string{"code": "UNSUPPORTED_MEDIA_TYPE", "message": "Content-Type must be application/json."})
	}
	var req service.OwnerRegistrationInput
	if err := decodeAuthBody(c, &req); err != nil {
		return authBodyError(c, err, "Registration request is invalid.")
	}
	result, err := h.registration.Register(c.Request().Context(), req)
	switch {
	case err == nil:
		return c.JSON(http.StatusCreated, result)
	case errors.Is(err, service.ErrInvalidOwnerInput):
		return badRequest(c, "INVALID_REQUEST", "Registration fields are invalid.")
	case errors.Is(err, service.ErrInvalidOwnerPassword):
		return badRequest(c, "INVALID_PASSWORD", "Password does not meet the password policy.")
	case errors.Is(err, timezone.ErrInvalid):
		return badRequest(c, "INVALID_TIMEZONE", "Outlet timezone is invalid.")
	case errors.Is(err, service.ErrOwnerEmailUnavailable):
		return c.JSON(http.StatusConflict, map[string]string{"code": "EMAIL_UNAVAILABLE", "message": "Registration cannot use this email."})
	default:
		return internalError(c)
	}
}
