package handlers

import (
	"errors"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
)

type CustomerHandler struct{ database *repository.Postgres }

func NewCustomerHandler(database *repository.Postgres) *CustomerHandler {
	return &CustomerHandler{database: database}
}

type customerInput struct {
	Name    string  `json:"name"`
	Phone   *string `json:"phone"`
	Address *string `json:"address"`
}

type customerResponse struct {
	ID         int64     `json:"id"`
	BusinessID int64     `json:"business_id"`
	Name       string    `json:"name"`
	Phone      *string   `json:"phone"`
	Address    *string   `json:"address"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type customerOrderResponse struct {
	ID            int64      `json:"id"`
	BusinessID    int64      `json:"business_id"`
	OutletID      int64      `json:"outlet_id"`
	CustomerID    int64      `json:"customer_id"`
	InvoiceNumber string     `json:"invoice_number"`
	Status        string     `json:"status"`
	PaymentStatus string     `json:"payment_status"`
	TotalAmount   int64      `json:"total_amount"`
	ReceivedAt    time.Time  `json:"received_at"`
	DueAt         *time.Time `json:"due_at"`
	CompletedAt   *time.Time `json:"completed_at"`
	CancelledAt   *time.Time `json:"cancelled_at"`
	CreatedAt     time.Time  `json:"created_at"`
}

func (h *CustomerHandler) List(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	page, size, offset, err := pagination(c)
	if err != nil {
		return badRequest(c, "INVALID_PAGINATION", err.Error())
	}
	search := strings.TrimSpace(c.QueryParam("q"))
	ctx := c.Request().Context()
	q := h.database.Queries()
	items, err := q.ListCustomers(ctx, postgresql.ListCustomersParams{BusinessID: p.BusinessID, Search: search, PageOffset: offset, PageLimit: size})
	if err != nil {
		return internalError(c)
	}
	total, err := q.CountCustomers(ctx, postgresql.CountCustomersParams{BusinessID: p.BusinessID, Search: search})
	if err != nil {
		return internalError(c)
	}
	response := make([]customerResponse, 0, len(items))
	for _, item := range items {
		response = append(response, customerResponseFrom(item.ID, item.BusinessID, item.Name, item.Phone, item.Address, item.CreatedAt, item.UpdatedAt))
	}
	return c.JSON(http.StatusOK, map[string]any{"items": response, "pagination": pageInfo(page, size, total)})
}

func (h *CustomerHandler) Get(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "customerId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Customer ID must be a positive integer.")
	}
	item, err := h.database.Queries().GetCustomer(c.Request().Context(), postgresql.GetCustomerParams{BusinessID: p.BusinessID, CustomerID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, customerResponseFrom(item.ID, item.BusinessID, item.Name, item.Phone, item.Address, item.CreatedAt, item.UpdatedAt))
}

func (h *CustomerHandler) Create(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	var req customerInput
	if err := decodeManagementJSON(c, &req); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Customer body is invalid.")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return badRequest(c, "INVALID_REQUEST", "Customer name is required.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	item, err := q.CreateCustomer(ctx, postgresql.CreateCustomerParams{BusinessID: p.BusinessID, Name: name, Phone: customerOptionalText(req.Phone), Address: customerOptionalText(req.Address)})
	if err != nil {
		return internalError(c)
	}
	if err := writeAudit(ctx, q, p, c, "CUSTOMER_CREATED", "customer", item.ID, nil, customerAuditValues(item.ID, item.Name, item.Phone, item.Address)); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusCreated, customerResponseFrom(item.ID, item.BusinessID, item.Name, item.Phone, item.Address, item.CreatedAt, item.UpdatedAt))
}

func (h *CustomerHandler) Update(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "customerId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Customer ID must be a positive integer.")
	}
	var req customerInput
	if err := decodeManagementJSON(c, &req); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Customer body is invalid.")
	}
	name := strings.TrimSpace(req.Name)
	if name == "" {
		return badRequest(c, "INVALID_REQUEST", "Customer name is required.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	old, err := q.GetCustomerForUpdate(ctx, postgresql.GetCustomerForUpdateParams{BusinessID: p.BusinessID, CustomerID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	item, err := q.UpdateCustomer(ctx, postgresql.UpdateCustomerParams{BusinessID: p.BusinessID, CustomerID: id, Name: name, Phone: customerOptionalText(req.Phone), Address: customerOptionalText(req.Address)})
	if err != nil {
		return internalError(c)
	}
	if err := writeAudit(ctx, q, p, c, "CUSTOMER_UPDATED", "customer", item.ID, customerAuditValues(old.ID, old.Name, old.Phone, old.Address), customerAuditValues(item.ID, item.Name, item.Phone, item.Address)); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, customerResponseFrom(item.ID, item.BusinessID, item.Name, item.Phone, item.Address, item.CreatedAt, item.UpdatedAt))
}

func (h *CustomerHandler) Deactivate(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "customerId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Customer ID must be a positive integer.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	old, err := q.GetCustomerForUpdate(ctx, postgresql.GetCustomerForUpdateParams{BusinessID: p.BusinessID, CustomerID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	item, err := q.DeactivateCustomer(ctx, postgresql.DeactivateCustomerParams{BusinessID: p.BusinessID, CustomerID: id})
	if err != nil {
		return internalError(c)
	}
	if err := writeAudit(ctx, q, p, c, "CUSTOMER_DEACTIVATED", "customer", item.ID, customerAuditValues(old.ID, old.Name, old.Phone, old.Address), map[string]any{"deleted": true}); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *CustomerHandler) OrderHistory(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "customerId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Customer ID must be a positive integer.")
	}
	page, size, offset, err := pagination(c)
	if err != nil {
		return badRequest(c, "INVALID_PAGINATION", err.Error())
	}
	ctx := c.Request().Context()
	q := h.database.Queries()
	if _, err := q.GetCustomerForHistory(ctx, postgresql.GetCustomerForHistoryParams{BusinessID: p.BusinessID, CustomerID: id}); errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	} else if err != nil {
		return internalError(c)
	}
	isAdmin := p.Role == "ADMIN"
	params := postgresql.ListCustomerOrderHistoryParams{BusinessID: p.BusinessID, CustomerID: id, IsAdmin: isAdmin, OutletIds: p.OutletIDs, PageOffset: offset, PageLimit: size}
	items, err := q.ListCustomerOrderHistory(ctx, params)
	if err != nil {
		return internalError(c)
	}
	total, err := q.CountCustomerOrderHistory(ctx, postgresql.CountCustomerOrderHistoryParams{BusinessID: p.BusinessID, CustomerID: id, IsAdmin: isAdmin, OutletIds: p.OutletIDs})
	if err != nil {
		return internalError(c)
	}
	response := make([]customerOrderResponse, 0, len(items))
	for _, item := range items {
		response = append(response, customerOrderResponse{
			ID: item.ID, BusinessID: item.BusinessID, OutletID: item.OutletID, CustomerID: item.CustomerID,
			InvoiceNumber: item.InvoiceNumber, Status: item.Status, PaymentStatus: item.PaymentStatus,
			TotalAmount: item.TotalAmount, ReceivedAt: item.ReceivedAt.Time, DueAt: nullableTimestamp(item.DueAt),
			CompletedAt: nullableTimestamp(item.CompletedAt), CancelledAt: nullableTimestamp(item.CancelledAt), CreatedAt: item.CreatedAt.Time,
		})
	}
	return c.JSON(http.StatusOK, map[string]any{"items": response, "pagination": pageInfo(page, size, total)})
}

func customerResponseFrom(id, businessID int64, name string, phone, address pgtype.Text, createdAt, updatedAt pgtype.Timestamptz) customerResponse {
	return customerResponse{ID: id, BusinessID: businessID, Name: name, Phone: nullableCustomerText(phone), Address: nullableCustomerText(address), CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time}
}

func customerOptionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	trimmed := strings.TrimSpace(*value)
	if trimmed == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: trimmed, Valid: true}
}

func customerAuditValues(id int64, _ string, _ pgtype.Text, _ pgtype.Text) map[string]any {
	return map[string]any{"id": id, "personal_data": "[REDACTED]"}
}

func nullableCustomerText(value pgtype.Text) *string {
	if !value.Valid {
		return nil
	}
	return &value.String
}

func nullableTimestamp(value pgtype.Timestamptz) *time.Time {
	if !value.Valid {
		return nil
	}
	return &value.Time
}
