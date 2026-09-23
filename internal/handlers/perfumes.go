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

type PerfumeHandler struct{ catalog *service.PerfumeCatalog }

func NewPerfumeHandler(database *repository.Postgres) *PerfumeHandler {
	return &PerfumeHandler{catalog: service.NewPerfumeCatalog(database)}
}

type perfumeRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"isActive"`
}

func (r perfumeRequest) input(update bool) (service.PerfumeInput, error) {
	if r.Name == nil || (update && r.IsActive == nil) {
		return service.PerfumeInput{}, errors.New("name is required and isActive is required for updates")
	}
	active := true
	if update {
		active = *r.IsActive
	} else if r.IsActive != nil {
		return service.PerfumeInput{}, errors.New("isActive is set by the server on creation")
	}
	return service.PerfumeInput{Name: *r.Name, Description: r.Description, IsActive: active}, nil
}

func perfumeActor(p Principal, c echo.Context) service.Actor {
	actor := service.Actor{BusinessID: p.BusinessID, UserID: p.UserID, UserAgent: c.Request().UserAgent()}
	if ip, err := netip.ParseAddr(c.RealIP()); err == nil {
		actor.IP = &ip
	}
	return actor
}

func perfumeError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidPerfumeName):
		return badRequest(c, "INVALID_PERFUME", err.Error())
	case errors.Is(err, service.ErrPerfumeNotFound):
		return notFound(c)
	case errors.Is(err, service.ErrPerfumeConflict):
		return conflict(c, "PERFUME_CONFLICT", err.Error())
	default:
		return internalError(c)
	}
}

func (h *PerfumeHandler) List(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	page, size, offset, err := pagination(c)
	if err != nil {
		return badRequest(c, "INVALID_PAGINATION", err.Error())
	}
	filter := service.PerfumeFilter{Search: c.QueryParam("q"), PageOffset: offset, PageLimit: size}
	if values, exists := c.QueryParams()["isActive"]; exists {
		if len(values) != 1 || (values[0] != "true" && values[0] != "false") {
			return badRequest(c, "INVALID_ACTIVE_FILTER", "isActive must be true or false")
		}
		active, _ := strconv.ParseBool(values[0])
		filter.IsActive = &active
	}
	items, total, err := h.catalog.List(c.Request().Context(), p.BusinessID, filter)
	if err != nil {
		return perfumeError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items, "pagination": pageInfo(page, size, total)})
}

func (h *PerfumeHandler) Get(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "perfumeId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Perfume ID must be a positive integer.")
	}
	item, err := h.catalog.Get(c.Request().Context(), p.BusinessID, id)
	if err != nil {
		return perfumeError(c, err)
	}
	return c.JSON(http.StatusOK, item)
}

func (h *PerfumeHandler) Create(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	var req perfumeRequest
	if err := decodeManagementJSON(c, &req); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Perfume body is invalid.")
	}
	input, err := req.input(false)
	if err != nil {
		return badRequest(c, "INVALID_REQUEST", err.Error())
	}
	item, err := h.catalog.Create(c.Request().Context(), perfumeActor(p, c), input)
	if err != nil {
		return perfumeError(c, err)
	}
	return c.JSON(http.StatusCreated, item)
}

func (h *PerfumeHandler) Update(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "perfumeId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Perfume ID must be a positive integer.")
	}
	var req perfumeRequest
	if err := decodeManagementJSON(c, &req); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Perfume body is invalid.")
	}
	input, err := req.input(true)
	if err != nil {
		return badRequest(c, "INVALID_REQUEST", err.Error())
	}
	item, err := h.catalog.Update(c.Request().Context(), perfumeActor(p, c), id, input)
	if err != nil {
		return perfumeError(c, err)
	}
	return c.JSON(http.StatusOK, item)
}

func (h *PerfumeHandler) Delete(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "perfumeId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Perfume ID must be a positive integer.")
	}
	if err := h.catalog.Delete(c.Request().Context(), perfumeActor(p, c), id); err != nil {
		return perfumeError(c, err)
	}
	return c.NoContent(http.StatusNoContent)
}
