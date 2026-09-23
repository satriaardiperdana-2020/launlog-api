package handlers

import (
	"crypto/sha256"
	"encoding/json"
	"errors"
	"net/http"

	"github.com/labstack/echo/v4"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/service"
)

type PaymentHandler struct{ catalog *service.PaymentCatalog }

func NewPaymentHandler(database *repository.Postgres) *PaymentHandler {
	return &PaymentHandler{catalog: service.NewPaymentCatalog(database)}
}

type recordPaymentRequest struct {
	Amount            *int64  `json:"amount"`
	Method            *string `json:"method"`
	ExternalReference *string `json:"externalReference"`
	Notes             *string `json:"notes"`
}

type voidPaymentRequest struct {
	Reason *string `json:"reason"`
}

func paymentError(c echo.Context, err error) error {
	switch {
	case errors.Is(err, service.ErrInvalidPayment):
		return badRequest(c, "INVALID_PAYMENT", err.Error())
	case errors.Is(err, service.ErrInvalidPaymentVoid):
		return badRequest(c, "INVALID_PAYMENT_VOID", err.Error())
	case errors.Is(err, service.ErrOrderOutletForbidden):
		return c.JSON(http.StatusForbidden, map[string]string{"code": "FORBIDDEN", "message": "The outlet is not assigned to this user."})
	case errors.Is(err, service.ErrPaymentNotFound):
		return notFound(c)
	case errors.Is(err, service.ErrPaymentOverpayment):
		return conflict(c, "OVERPAYMENT", err.Error())
	case errors.Is(err, service.ErrPaymentNoBalance):
		return conflict(c, "NO_OUTSTANDING_BALANCE", err.Error())
	case errors.Is(err, service.ErrPaymentOrderCancelled):
		return conflict(c, "ORDER_CANCELLED", err.Error())
	case errors.Is(err, service.ErrPaymentNotVoidable):
		return conflict(c, "PAYMENT_NOT_VOIDABLE", err.Error())
	case errors.Is(err, service.ErrPaymentRefundsUnsupported):
		return conflict(c, "REFUND_POLICY_REQUIRED", err.Error())
	case errors.Is(err, service.ErrPaymentIdempotencyConflict):
		return conflict(c, "IDEMPOTENCY_CONFLICT", err.Error())
	default:
		return internalError(c)
	}
}

func (h *PaymentHandler) Void(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	orderID, err := pathID(c, "orderId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Order ID must be a positive integer.")
	}
	paymentID, err := pathID(c, "paymentId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Payment ID must be a positive integer.")
	}
	var req voidPaymentRequest
	if err := decodeManagementJSON(c, &req); err != nil || req.Reason == nil {
		return badRequest(c, "INVALID_PAYMENT_VOID", "A void reason is required.")
	}
	result, err := h.catalog.Void(c.Request().Context(), orderActor(p, c), orderID, paymentID, *req.Reason)
	if err != nil {
		return paymentError(c, err)
	}
	return c.JSON(http.StatusOK, result)
}

func (h *PaymentHandler) Record(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	orderID, err := pathID(c, "orderId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Order ID must be a positive integer.")
	}
	key := c.Request().Header.Get("Idempotency-Key")
	if !orderIdempotencyKeyPattern.MatchString(key) {
		return badRequest(c, "INVALID_IDEMPOTENCY_KEY", "Idempotency-Key must contain 1 to 128 letters, digits, period, underscore, colon, or hyphen.")
	}
	var req recordPaymentRequest
	if err := decodeManagementJSON(c, &req); err != nil || req.Amount == nil || req.Method == nil {
		return badRequest(c, "INVALID_PAYMENT", "amount and method are required.")
	}
	canonical, err := json.Marshal(req)
	if err != nil {
		return badRequest(c, "INVALID_PAYMENT", "Payment body is invalid.")
	}
	hash := sha256.Sum256(canonical)
	result, err := h.catalog.Record(c.Request().Context(), orderActor(p, c), orderID, service.RecordPaymentInput{
		TenderedAmount: *req.Amount, Method: *req.Method, ExternalReference: req.ExternalReference,
		Notes: req.Notes, IdempotencyKey: key, RequestHash: hash[:],
	})
	if err != nil {
		return paymentError(c, err)
	}
	if result.Replayed {
		c.Response().Header().Set("Idempotency-Replayed", "true")
	}
	return c.JSON(http.StatusCreated, result)
}

func (h *PaymentHandler) List(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	orderID, err := pathID(c, "orderId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Order ID must be a positive integer.")
	}
	page, size, offset, err := pagination(c)
	if err != nil {
		return badRequest(c, "INVALID_PAGINATION", err.Error())
	}
	result, err := h.catalog.List(c.Request().Context(), orderActor(p, c), orderID, offset, size)
	if err != nil {
		return paymentError(c, err)
	}
	return c.JSON(http.StatusOK, map[string]any{
		"order_id": result.OrderID, "order_total": result.OrderTotal, "payment_status": result.PaymentStatus,
		"paid_amount": result.PaidAmount, "outstanding_amount": result.OutstandingAmount,
		"items": result.Items, "pagination": pageInfo(page, size, result.TotalItems),
	})
}
