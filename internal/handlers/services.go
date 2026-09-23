package handlers

import (
	"errors"
	"net/http"
	"net/netip"
	"strconv"

	"github.com/labstack/echo/v4"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/service"
)

type ServiceHandler struct{ catalog *service.ServiceCatalog }

func NewServiceHandler(database *repository.Postgres) *ServiceHandler {
	return &ServiceHandler{catalog: service.NewServiceCatalog(database)}
}

type serviceRequest struct {
	Name                     *string `json:"name"`
	Description              *string `json:"description"`
	Unit                     *string `json:"unit"`
	UnitPriceAmount          *int64  `json:"unitPriceAmount"`
	EstimatedDurationMinutes *int32  `json:"estimatedDurationMinutes"`
	IsActive                 *bool   `json:"isActive"`
}

func (r serviceRequest) input(update bool) (service.ServiceInput, error) {
	if r.Name == nil || r.Unit == nil || r.UnitPriceAmount == nil || (update && (r.EstimatedDurationMinutes == nil || r.IsActive == nil)) {
		return service.ServiceInput{}, errors.New("name, unit, unitPriceAmount, and (for updates) estimatedDurationMinutes and isActive are required")
	}
	duration := int32(0)
	if r.EstimatedDurationMinutes != nil {
		duration = *r.EstimatedDurationMinutes
	}
	active := true
	if update {
		active = *r.IsActive
	}
	if !update && r.IsActive != nil {
		return service.ServiceInput{}, errors.New("isActive is set by the server on creation")
	}
	return service.ServiceInput{Name: *r.Name, Description: r.Description, Unit: *r.Unit, UnitPriceAmount: *r.UnitPriceAmount, EstimatedDurationMinutes: duration, IsActive: active}, nil
}

func serviceActor(p Principal, c echo.Context) service.Actor {
	actor := service.Actor{BusinessID: p.BusinessID, UserID: p.UserID, UserAgent: c.Request().UserAgent()}
	if ip, err := netip.ParseAddr(c.RealIP()); err == nil {
		actor.IP = &ip
	}
	return actor
}

func serviceError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidName), errors.Is(err, service.ErrInvalidUnit), errors.Is(err, service.ErrInvalidPrice), errors.Is(err, service.ErrInvalidDuration):
		return badRequest(c, "INVALID_SERVICE", err.Error())
	case errors.Is(err, service.ErrNotFound):
		return notFound(c)
	case errors.Is(err, service.ErrConflict):
		return conflict(c, "SERVICE_CONFLICT", err.Error())
	default:
		return internalError(c)
	}
}

func (h *ServiceHandler) List(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	page, size, offset, err := pagination(c)
	if err != nil {
		return badRequest(c, "INVALID_PAGINATION", err.Error())
	}
	filter := service.ServiceFilter{Search: c.QueryParam("q"), PageOffset: offset, PageLimit: size}
	if values, exists := c.QueryParams()["unit"]; exists {
		if len(values) != 1 || !service.ValidUnit(values[0]) {
			return badRequest(c, "INVALID_UNIT", "unit must be KILOGRAM, PIECE, METER, or SQUARE_METER")
		}
		filter.Unit = &values[0]
	}
	if values, exists := c.QueryParams()["isActive"]; exists {
		if len(values) != 1 || (values[0] != "true" && values[0] != "false") {
			return badRequest(c, "INVALID_ACTIVE_FILTER", "isActive must be true or false")
		}
		active, _ := strconv.ParseBool(values[0])
		filter.IsActive = &active
	}
	items, total, err := h.catalog.List(c.Request().Context(), p.BusinessID, filter)
	if err != nil {
		return serviceError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items, "pagination": pageInfo(page, size, total)})
}

func (h *ServiceHandler) Get(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "serviceId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Service ID must be a positive integer.")
	}
	item, err := h.catalog.Get(c.Request().Context(), p.BusinessID, id)
	if err != nil {
		return serviceError(c, err)
	}
	return c.JSON(http.StatusOK, item)
}

func (h *ServiceHandler) Create(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	var req serviceRequest
	if err := decodeManagementJSON(c, &req); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Service body is invalid.")
	}
	input, err := req.input(false)
	if err != nil {
		return badRequest(c, "INVALID_REQUEST", err.Error())
	}
	item, err := h.catalog.Create(c.Request().Context(), serviceActor(p, c), input)
	if err != nil {
		return serviceError(c, err)
	}
	return c.JSON(http.StatusCreated, item)
}

func (h *ServiceHandler) Update(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "serviceId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Service ID must be a positive integer.")
	}
	var req serviceRequest
	if err := decodeManagementJSON(c, &req); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Service body is invalid.")
	}
	input, err := req.input(true)
	if err != nil {
		return badRequest(c, "INVALID_REQUEST", err.Error())
	}
	item, err := h.catalog.Update(c.Request().Context(), serviceActor(p, c), id, input)
	if err != nil {
		return serviceError(c, err)
	}
	return c.JSON(http.StatusOK, item)
}

func (h *ServiceHandler) Delete(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "serviceId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Service ID must be a positive integer.")
	}
	if err := h.catalog.Delete(c.Request().Context(), serviceActor(p, c), id); err != nil {
		return serviceError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
