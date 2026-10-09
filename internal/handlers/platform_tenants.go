package handlers

import (
	"errors"
	"net/http"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/service"
	"github.com/satriaardiperdana-2020/launlog-api/internal/timezone"
)

type PlatformTenantHandler struct{ catalog *service.PlatformTenants }

func NewPlatformTenantHandler(d *repository.Postgres) *PlatformTenantHandler {
	return &PlatformTenantHandler{catalog: service.NewPlatformTenants(d)}
}
func platformActor(c echo.Context) service.PlatformActor {
	p, _ := PlatformPrincipalFromContext(c)
	id := c.Response().Header().Get(echo.HeaderXRequestID)
	if len(id) > 128 {
		id = id[:128]
	}
	return service.PlatformActor{ID: p.ID, SessionID: p.SessionID, RequestID: id}
}
func platformResult(c echo.Context, result any, err error, status int) error {
	switch {
	case err == nil:
		return c.JSON(status, result)
	case errors.Is(err, timezone.ErrInvalid):
		return badRequest(c, "INVALID_TIMEZONE", err.Error())
	case errors.Is(err, service.ErrPlatformInvalid):
		return badRequest(c, "INVALID_REQUEST", "Request is invalid.")
	case errors.Is(err, service.ErrPlatformDenied):
		return c.JSON(http.StatusForbidden, map[string]string{"code": "SUPPORT_ACCESS_DENIED", "message": "This operation is not authorized."})
	case errors.Is(err, service.ErrPlatformNotFound):
		return c.JSON(http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "Resource was not found."})
	case errors.Is(err, service.ErrAdminEmailUnavailable):
		return conflict(c, "ADMIN_EMAIL_UNAVAILABLE", "Administrator email is unavailable.")
	case errors.Is(err, service.ErrPlatformConflict):
		return conflict(c, "RESOURCE_CONFLICT", "Operation conflicts with current state.")
	default:
		return internalError(c)
	}
}
func (h *PlatformTenantHandler) Provision(c echo.Context) error {
	var in service.ProvisionBusinessInput
	if err := decodeManagementJSON(c, &in); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Business body is invalid.")
	}
	r, err := h.catalog.Provision(c.Request().Context(), platformActor(c), in)
	return platformResult(c, r, err, http.StatusCreated)
}
func (h *PlatformTenantHandler) Businesses(c echo.Context) error {
	id := int64(0)
	var err error
	if c.Param("businessId") != "" {
		id, err = pathID(c, "businessId")
		if err != nil {
			return badRequest(c, "INVALID_REQUEST", "Invalid business ID.")
		}
	}
	_, limit, offset, err := pagination(c)
	if err != nil {
		return badRequest(c, "INVALID_REQUEST", "Invalid pagination.")
	}
	active := pgtype.Bool{}
	if c.QueryParam("isActive") != "" {
		v, e := strconv.ParseBool(c.QueryParam("isActive"))
		if e != nil {
			return badRequest(c, "INVALID_REQUEST", "Invalid active filter.")
		}
		active = pgtype.Bool{Bool: v, Valid: true}
	}
	r, err := h.catalog.Businesses(c.Request().Context(), platformActor(c), id, c.QueryParam("q"), active, limit, offset)
	return platformResult(c, r, err, http.StatusOK)
}
func (h *PlatformTenantHandler) Activation(c echo.Context) error {
	id, err := pathID(c, "businessId")
	if err != nil {
		return badRequest(c, "INVALID_REQUEST", "Invalid business ID.")
	}
	var in struct {
		IsActive *bool  `json:"isActive"`
		Reason   string `json:"reason"`
	}
	if err := decodeManagementJSON(c, &in); err != nil || in.IsActive == nil {
		return badRequest(c, "INVALID_REQUEST", "Activation body is invalid.")
	}
	r, err := h.catalog.Activation(c.Request().Context(), platformActor(c), id, *in.IsActive, in.Reason)
	return platformResult(c, r, err, http.StatusOK)
}
func (h *PlatformTenantHandler) RequestSupport(c echo.Context) error {
	p, _ := PrincipalFromContext(c)
	var in service.SupportRequestInput
	if err := decodeManagementJSON(c, &in); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Support request body is invalid.")
	}
	r, err := h.catalog.RequestSupport(c.Request().Context(), p.BusinessID, p.UserID, platformActor(c).RequestID, in)
	return platformResult(c, r, err, http.StatusCreated)
}
func (h *PlatformTenantHandler) StartSupport(c echo.Context) error {
	id, err := pathID(c, "businessId")
	if err != nil {
		return badRequest(c, "INVALID_REQUEST", "Invalid business ID.")
	}
	var in struct {
		SupportRequestID int64 `json:"supportRequestId"`
	}
	if err := decodeManagementJSON(c, &in); err != nil || in.SupportRequestID < 1 {
		return badRequest(c, "INVALID_REQUEST", "Support session body is invalid.")
	}
	r, err := h.catalog.StartSupport(c.Request().Context(), platformActor(c), id, in.SupportRequestID)
	return platformResult(c, r, err, http.StatusCreated)
}
func (h *PlatformTenantHandler) EndSupport(request bool, owner bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		business := int64(0)
		user := int64(0)
		a := platformActor(c)
		var err error
		if owner {
			p, _ := PrincipalFromContext(c)
			business = p.BusinessID
			user = p.UserID
		} else if c.Param("businessId") != "" {
			business, err = pathID(c, "businessId")
			if err != nil {
				return badRequest(c, "INVALID_REQUEST", "Invalid business ID.")
			}
		}
		key := "sessionId"
		if request {
			key = "requestId"
		}
		id, err := pathID(c, key)
		if err != nil {
			return badRequest(c, "INVALID_REQUEST", "Invalid target ID.")
		}
		var in struct {
			Reason string `json:"reason"`
		}
		if err := decodeManagementJSON(c, &in); err != nil {
			return badRequest(c, "INVALID_REQUEST", "Reason body is invalid.")
		}
		r, err := h.catalog.EndSupport(c.Request().Context(), a, user, business, id, request, in.Reason)
		return platformResult(c, r, err, http.StatusOK)
	}
}
func (h *PlatformTenantHandler) View(kind string, owner bool) echo.HandlerFunc {
	return func(c echo.Context) error {
		business := int64(0)
		user := int64(0)
		a := platformActor(c)
		var err error
		if owner {
			p, _ := PrincipalFromContext(c)
			business = p.BusinessID
			user = p.UserID
		} else if c.Param("businessId") != "" {
			business, err = pathID(c, "businessId")
			if err != nil {
				return badRequest(c, "INVALID_REQUEST", "Invalid business ID.")
			}
		}
		_, limit, offset, err := pagination(c)
		if err != nil {
			return badRequest(c, "INVALID_REQUEST", "Invalid pagination.")
		}
		target := int64(0)
		if kind == "request" {
			target, err = pathID(c, "requestId")
			if err != nil {
				return badRequest(c, "INVALID_REQUEST", "Invalid request ID.")
			}
		}
		r, err := h.catalog.Views(c.Request().Context(), a, user, business, target, kind, limit, offset)
		return platformResult(c, r, err, http.StatusOK)
	}
}
func (h *PlatformTenantHandler) SupportOperation(operation string) echo.HandlerFunc {
	return func(c echo.Context) error {
		business, err := pathID(c, "businessId")
		if err != nil {
			return badRequest(c, "INVALID_REQUEST", "Invalid business ID.")
		}
		session, err := pathID(c, "sessionId")
		if err != nil {
			return badRequest(c, "INVALID_REQUEST", "Invalid session ID.")
		}
		target := int64(0)
		for _, key := range []string{"outletId", "perfumeId"} {
			if c.Param(key) != "" {
				target, err = pathID(c, key)
				if err != nil {
					return badRequest(c, "INVALID_REQUEST", "Invalid record ID.")
				}
			}
		}
		_, limit, offset, err := pagination(c)
		if err != nil {
			return badRequest(c, "INVALID_REQUEST", "Invalid pagination.")
		}
		description := ""
		version := int64(0)
		if operation == "PERFUME_DESCRIPTION_UPDATED" {
			var in struct {
				Description *string `json:"description"`
			}
			if err := decodeManagementJSON(c, &in); err != nil || in.Description == nil {
				return badRequest(c, "INVALID_REQUEST", "Description body is invalid.")
			}
			description = *in.Description
			version, err = strconv.ParseInt(strings.Trim(c.Request().Header.Get("If-Match"), "\""), 10, 64)
			if err != nil || version < 1 {
				return badRequest(c, "INVALID_REQUEST", "A valid If-Match version is required.")
			}
		}
		r, err := h.catalog.SupportOperation(c.Request().Context(), platformActor(c), business, session, target, operation, description, version, limit, offset)
		return platformResult(c, r, err, http.StatusOK)
	}
}

func (h *PlatformTenantHandler) BusinessAudit(c echo.Context) error {
	p, _ := PrincipalFromContext(c)
	_, limit, offset, err := pagination(c)
	if err != nil {
		return badRequest(c, "INVALID_REQUEST", "Invalid pagination.")
	}
	r, err := h.catalog.BusinessAudit(c.Request().Context(), p.BusinessID, p.UserID, p.Role, limit, offset)
	return platformResult(c, r, err, http.StatusOK)
}
