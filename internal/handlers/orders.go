package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"
	"net/netip"
	"regexp"
	"strconv"
	"time"

	"github.com/labstack/echo/v4"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/service"
)

var orderIdempotencyKeyPattern = regexp.MustCompile(`^[A-Za-z0-9._:-]{1,128}$`)

type OrderHandler struct{ catalog *service.OrderCatalog }

func NewOrderHandler(database *repository.Postgres) *OrderHandler {
	return &OrderHandler{catalog: service.NewOrderCatalog(database)}
}

type createOrderRequest struct {
	OutletID   *int64                   `json:"outletId"`
	CustomerID *int64                   `json:"customerId"`
	PerfumeID  *int64                   `json:"perfumeId"`
	DueAt      *string                  `json:"dueAt"`
	Notes      *string                  `json:"notes"`
	Items      []createOrderItemRequest `json:"items"`
}

type createOrderItemRequest struct {
	ServiceID *int64  `json:"serviceId"`
	Quantity  *string `json:"quantity"`
	Notes     *string `json:"notes"`
}

func orderActor(p Principal, c echo.Context) service.Actor {
	actor := service.Actor{BusinessID: p.BusinessID, UserID: p.UserID, Role: p.Role, OutletIDs: p.OutletIDs, UserAgent: c.Request().UserAgent()}
	if ip, err := netip.ParseAddr(c.RealIP()); err == nil {
		actor.IP = &ip
	}
	return actor
}

func orderError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidOrder), errors.Is(err, service.ErrOrderTotalOverflow):
		return badRequest(c, "INVALID_ORDER", err.Error())
	case errors.Is(err, service.ErrOrderOutletForbidden):
		return c.JSON(http.StatusForbidden, map[string]string{"code": "FORBIDDEN", "message": "The outlet is not assigned to this user."})
	case errors.Is(err, service.ErrOrderIdempotencyConflict):
		return conflict(c, "IDEMPOTENCY_CONFLICT", err.Error())
	case errors.Is(err, service.ErrOrderNotFound), errors.Is(err, service.ErrOrderCustomerNotFound), errors.Is(err, service.ErrOrderServiceNotFound), errors.Is(err, service.ErrOrderPerfumeNotFound):
		return notFound(c)
	default:
		return internalError(c)
	}
}

func (h *OrderHandler) Create(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	key := c.Request().Header.Get("Idempotency-Key")
	if !orderIdempotencyKeyPattern.MatchString(key) {
		return badRequest(c, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must contain 1 to 128 letters, digits, period, underscore, colon, or hyphen.")
	}
	var req createOrderRequest
	if err := decodeManagementJSON(c, &req); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Order body is invalid.")
	}
	if req.OutletID == nil || req.CustomerID == nil || len(req.Items) == 0 {
		return badRequest(c, "INVALID_ORDER", "outletId, customerId, and at least one item are required.")
	}
	var dueAt *time.Time
	if req.DueAt != nil {
		parsed, err := time.Parse(time.RFC3339Nano, *req.DueAt)
		if err != nil {
			return badRequest(c, "INVALID_DUE_AT", "dueAt must be an RFC 3339 date-time with an explicit offset.")
		}
		dueAt = &parsed
	}
	items := make([]service.OrderItemInput, 0, len(req.Items))
	for _, item := range req.Items {
		if item.ServiceID == nil || item.Quantity == nil {
			return badRequest(c, "INVALID_ORDER", "Each item requires serviceId and decimal-string quantity.")
		}
		items = append(items, service.OrderItemInput{ServiceID: *item.ServiceID, Quantity: *item.Quantity, Notes: item.Notes})
	}
	canonicalPayload, err := json.Marshal(req)
	if err != nil {
		return badRequest(c, "INVALID_REQUEST", "Order body is invalid.")
	}
	hash := sha256.Sum256(canonicalPayload)
	result, err := h.catalog.Create(c.Request().Context(), orderActor(p, c), service.CreateOrderInput{
		OutletID: *req.OutletID, CustomerID: *req.CustomerID, PerfumeID: req.PerfumeID,
		DueAt: dueAt, Notes: req.Notes, Items: items, IdempotencyKey: key, RequestHash: hash[:],
	})
	if err != nil {
		return orderError(c, err)
	}
	if result.Replayed {
		c.Response().Header().Set("Idempotency-Replayed", "true")
	}
	return c.JSON(http.StatusCreated, result.Order)
}

func (h *OrderHandler) Get(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "orderId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Order ID must be a positive integer.")
	}
	order, err := h.catalog.Get(c.Request().Context(), orderActor(p, c), id)
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(http.StatusOK, order)
}

func (h *OrderHandler) List(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	page, size, offset, err := pagination(c)
	if err != nil {
		return badRequest(c, "INVALID_PAGINATION", err.Error())
	}
	filter := service.OrderFilter{PageOffset: offset, PageLimit: size}
	if raw := c.QueryParam("outletId"); raw != "" {
		id, err := strconv.ParseInt(raw, 10, 64)
		if err != nil || id < 1 {
			return badRequest(c, "INVALID_OUTLET_ID", "outletId must be a positive integer.")
		}
		if p.Role != "ADMIN" && !orderHasOutlet(p.OutletIDs, id) {
			return c.JSON(http.StatusForbidden, map[string]string{"code": "FORBIDDEN", "message": "The outlet is not assigned to this user."})
		}
		filter.OutletID = &id
	}
	if raw := c.QueryParam("status"); raw != "" {
		switch raw {
		case "RECEIVED", "PROCESSING", "READY_FOR_PICKUP", "COMPLETED", "CANCELLED":
			filter.Status = &raw
		default:
			return badRequest(c, "INVALID_STATUS", "status is not a supported order status.")
		}
	}
	orders, total, err := h.catalog.List(c.Request().Context(), orderActor(p, c), filter)
	if err != nil {
		return orderError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": orders, "pagination": pageInfo(page, size, total)})
}

func orderHasOutlet(outlets []int64, outletID int64) bool {
	for _, id := range outlets {
		if id == outletID {
			return true
		}
	}
	return false
}
