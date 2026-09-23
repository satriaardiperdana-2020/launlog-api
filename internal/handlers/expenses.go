package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
)

type ExpenseHandler struct{ database *repository.Postgres }

func NewExpenseHandler(database *repository.Postgres) *ExpenseHandler {
	return &ExpenseHandler{database: database}
}

type expenseCategoryRequest struct {
	Name        *string `json:"name"`
	Description *string `json:"description"`
	IsActive    *bool   `json:"isActive"`
}

type expenseRequest struct {
	OutletID         *int64     `json:"outletId"`
	CategoryID       *int64     `json:"categoryId"`
	Amount           *int64     `json:"amount"`
	Description      *string    `json:"description"`
	ExpenseAt        *time.Time `json:"expenseAt"`
	ReceiptReference *string    `json:"receiptReference"`
}

type expenseCategoryResponse struct {
	ID          int64     `json:"id"`
	BusinessID  int64     `json:"business_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type expenseResponse struct {
	ID               int64     `json:"id"`
	BusinessID       int64     `json:"business_id"`
	OutletID         int64     `json:"outlet_id"`
	CategoryID       int64     `json:"category_id"`
	Amount           int64     `json:"amount"`
	Description      string    `json:"description"`
	ExpenseAt        time.Time `json:"expense_at"`
	ReceiptReference *string   `json:"receipt_reference"`
	CreatedBy        int64     `json:"created_by"`
	CreatedAt        time.Time `json:"created_at"`
	UpdatedAt        time.Time `json:"updated_at"`
	Version          int64     `json:"version"`
}

func (h *ExpenseHandler) ListCategories(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	page, size, offset, err := pagination(c)
	if err != nil {
		return badRequest(c, "INVALID_PAGINATION", err.Error())
	}
	includeInactive := false
	if raw := c.QueryParam("includeInactive"); raw != "" {
		if raw != "true" && raw != "false" {
			return badRequest(c, "INVALID_FILTER", "includeInactive must be true or false.")
		}
		includeInactive = raw == "true"
	}
	ctx := c.Request().Context()
	q := h.database.Queries()
	args := postgresql.ListExpenseCategoriesParams{BusinessID: p.BusinessID, Search: strings.TrimSpace(c.QueryParam("q")), IncludeInactive: includeInactive, PageOffset: offset, PageLimit: size}
	items, err := q.ListExpenseCategories(ctx, args)
	if err != nil {
		return internalError(c)
	}
	total, err := q.CountExpenseCategories(ctx, postgresql.CountExpenseCategoriesParams{BusinessID: p.BusinessID, Search: args.Search, IncludeInactive: includeInactive})
	if err != nil {
		return internalError(c)
	}
	result := make([]expenseCategoryResponse, 0, len(items))
	for _, item := range items {
		result = append(result, expenseCategoryFrom(item))
	}
	return c.JSON(http.StatusOK, map[string]any{"items": result, "pagination": pageInfo(page, size, total)})
}

func (h *ExpenseHandler) GetCategory(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "categoryId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Category ID must be a positive integer.")
	}
	item, err := h.database.Queries().GetExpenseCategory(c.Request().Context(), postgresql.GetExpenseCategoryParams{BusinessID: p.BusinessID, CategoryID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, expenseCategoryFrom(item))
}

func (h *ExpenseHandler) CreateCategory(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	var req expenseCategoryRequest
	if err := decodeManagementJSON(c, &req); err != nil || req.Name == nil {
		return badRequest(c, "INVALID_REQUEST", "Category name is required.")
	}
	name := strings.TrimSpace(*req.Name)
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return badRequest(c, "INVALID_REQUEST", "Category name is required.")
	}
	if req.Description != nil && utf8.RuneCountInString(*req.Description) > 1000 {
		return badRequest(c, "INVALID_REQUEST", "Category description cannot exceed 1000 characters.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	item, err := q.CreateExpenseCategory(ctx, postgresql.CreateExpenseCategoryParams{BusinessID: p.BusinessID, Name: name, Description: optionalDBText(req.Description)})
	if err != nil {
		return expenseWriteError(c, err)
	}
	response := expenseCategoryFrom(item)
	if err := writeExpenseCategoryAudit(ctx, q, p, c, "EXPENSE_CATEGORY_CREATED", item.ID, nil, categoryAuditValues(response)); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusCreated, response)
}

func (h *ExpenseHandler) UpdateCategory(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "categoryId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Category ID must be a positive integer.")
	}
	var req expenseCategoryRequest
	if err := decodeManagementJSON(c, &req); err != nil || req.Name == nil || req.IsActive == nil {
		return badRequest(c, "INVALID_REQUEST", "name and isActive are required.")
	}
	name := strings.TrimSpace(*req.Name)
	if name == "" || utf8.RuneCountInString(name) > 200 {
		return badRequest(c, "INVALID_REQUEST", "Category name is required.")
	}
	if req.Description != nil && utf8.RuneCountInString(*req.Description) > 1000 {
		return badRequest(c, "INVALID_REQUEST", "Category description cannot exceed 1000 characters.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	old, err := q.GetExpenseCategoryForUpdate(ctx, postgresql.GetExpenseCategoryForUpdateParams{BusinessID: p.BusinessID, CategoryID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	item, err := q.UpdateExpenseCategory(ctx, postgresql.UpdateExpenseCategoryParams{BusinessID: p.BusinessID, CategoryID: id, Name: name, Description: optionalDBText(req.Description), IsActive: *req.IsActive})
	if err != nil {
		return expenseWriteError(c, err)
	}
	before, after := expenseCategoryFrom(old), expenseCategoryFrom(item)
	action := "EXPENSE_CATEGORY_UPDATED"
	if before.IsActive && !after.IsActive {
		action = "EXPENSE_CATEGORY_DEACTIVATED"
	} else if !before.IsActive && after.IsActive {
		action = "EXPENSE_CATEGORY_REACTIVATED"
	}
	if err := writeExpenseCategoryAudit(ctx, q, p, c, action, id, categoryAuditValues(before), categoryAuditValues(after)); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, after)
}

func (h *ExpenseHandler) List(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	page, size, offset, err := pagination(c)
	if err != nil {
		return badRequest(c, "INVALID_PAGINATION", err.Error())
	}
	from, to, err := expenseDateRange(c)
	if err != nil {
		return badRequest(c, "INVALID_DATE_RANGE", err.Error())
	}
	var outletID pgtype.Int8
	if raw := c.QueryParam("outletId"); raw != "" {
		id, parseErr := strconv.ParseInt(raw, 10, 64)
		if parseErr != nil || id <= 0 {
			return badRequest(c, "INVALID_OUTLET", "outletId must be a positive integer.")
		}
		outletID = pgtype.Int8{Int64: id, Valid: true}
	}
	ctx := c.Request().Context()
	q := h.database.Queries()
	args := postgresql.ListExpensesParams{BusinessID: p.BusinessID, IsAdmin: p.Role == "ADMIN", OutletIds: p.OutletIDs, OutletID: outletID, DateFrom: dateParam(from), DateTo: dateParam(to), PageOffset: offset, PageLimit: size}
	items, err := q.ListExpenses(ctx, args)
	if err != nil {
		return internalError(c)
	}
	total, err := q.CountExpenses(ctx, postgresql.CountExpensesParams{BusinessID: p.BusinessID, IsAdmin: p.Role == "ADMIN", OutletIds: p.OutletIDs, OutletID: outletID, DateFrom: dateParam(from), DateTo: dateParam(to)})
	if err != nil {
		return internalError(c)
	}
	result := make([]expenseResponse, 0, len(items))
	for _, item := range items {
		result = append(result, expenseFrom(item))
	}
	return c.JSON(http.StatusOK, map[string]any{"items": result, "pagination": pageInfo(page, size, total), "date_from": from.Format("2006-01-02"), "date_to": to.Format("2006-01-02"), "timezone": "Asia/Jakarta"})
}

func (h *ExpenseHandler) Get(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "expenseId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Expense ID must be a positive integer.")
	}
	item, err := h.database.Queries().GetExpense(c.Request().Context(), postgresql.GetExpenseParams{BusinessID: p.BusinessID, ExpenseID: id, IsAdmin: p.Role == "ADMIN", UserID: p.UserID})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	r := expenseFrom(item)
	c.Response().Header().Set("ETag", expenseETag(r.Version))
	return c.JSON(http.StatusOK, r)
}

func (h *ExpenseHandler) Create(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	var req expenseRequest
	if err := decodeManagementJSON(c, &req); err != nil || req.OutletID == nil || req.CategoryID == nil || req.Amount == nil || req.Description == nil {
		return badRequest(c, "INVALID_REQUEST", "outletId, categoryId, amount, and description are required.")
	}
	if *req.OutletID <= 0 || *req.CategoryID <= 0 || *req.Amount <= 0 {
		return badRequest(c, "INVALID_EXPENSE", "outletId/categoryId and amount must be positive.")
	}
	desc := strings.TrimSpace(*req.Description)
	if desc == "" || utf8.RuneCountInString(desc) > 2000 {
		return badRequest(c, "INVALID_EXPENSE", "description must contain 1 to 2000 characters.")
	}
	if req.ReceiptReference != nil && utf8.RuneCountInString(*req.ReceiptReference) > 256 {
		return badRequest(c, "INVALID_EXPENSE", "receiptReference cannot exceed 256 characters.")
	}
	expenseAt := time.Now().UTC()
	if req.ExpenseAt != nil {
		expenseAt = req.ExpenseAt.UTC()
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	if _, err := q.LockActiveExpenseOutlet(ctx, postgresql.LockActiveExpenseOutletParams{BusinessID: p.BusinessID, OutletID: *req.OutletID, UserID: p.UserID, IsAdmin: p.Role == "ADMIN"}); errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	} else if err != nil {
		return internalError(c)
	}
	if p.Role != "ADMIN" {
		if _, err := q.LockExpenseOutletAssignment(ctx, postgresql.LockExpenseOutletAssignmentParams{BusinessID: p.BusinessID, UserID: p.UserID, OutletID: *req.OutletID}); errors.Is(err, pgx.ErrNoRows) {
			return notFound(c)
		} else if err != nil {
			return internalError(c)
		}
	}
	if _, err := q.GetActiveExpenseCategory(ctx, postgresql.GetActiveExpenseCategoryParams{BusinessID: p.BusinessID, CategoryID: *req.CategoryID}); errors.Is(err, pgx.ErrNoRows) {
		return conflict(c, "INACTIVE_EXPENSE_CATEGORY", "Expense category is unavailable or inactive.")
	} else if err != nil {
		return internalError(c)
	}
	item, err := q.CreateExpense(ctx, postgresql.CreateExpenseParams{BusinessID: p.BusinessID, OutletID: *req.OutletID, CategoryID: *req.CategoryID, Amount: *req.Amount, Description: desc, ExpenseAt: pgtype.Timestamptz{Time: expenseAt, Valid: true}, ReceiptReference: optionalDBText(req.ReceiptReference), UserID: p.UserID, IsAdmin: p.Role == "ADMIN"})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return expenseWriteError(c, err)
	}
	r := expenseFrom(item)
	if err := writeExpenseAudit(ctx, q, p, c, "EXPENSE_CREATED", r.ID, r.OutletID, nil, expenseAuditValues(r)); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	c.Response().Header().Set("ETag", expenseETag(r.Version))
	return c.JSON(http.StatusCreated, r)
}

func (h *ExpenseHandler) Update(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	id, err := pathID(c, "expenseId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Expense ID must be a positive integer.")
	}
	expected, err := parseExpenseETag(c.Request().Header.Get("If-Match"))
	if err != nil {
		return c.JSON(http.StatusPreconditionRequired, map[string]string{"code": "PRECONDITION_REQUIRED", "message": "A valid If-Match expense version is required."})
	}
	var req expenseRequest
	if err := decodeManagementJSON(c, &req); err != nil || req.CategoryID == nil || req.Amount == nil || req.Description == nil || req.ExpenseAt == nil {
		return badRequest(c, "INVALID_REQUEST", "categoryId, amount, description, and expenseAt are required.")
	}
	if *req.CategoryID <= 0 || *req.Amount <= 0 {
		return badRequest(c, "INVALID_EXPENSE", "categoryId and amount must be positive.")
	}
	desc := strings.TrimSpace(*req.Description)
	if desc == "" || utf8.RuneCountInString(desc) > 2000 {
		return badRequest(c, "INVALID_EXPENSE", "description must contain 1 to 2000 characters.")
	}
	if req.ReceiptReference != nil && utf8.RuneCountInString(*req.ReceiptReference) > 256 {
		return badRequest(c, "INVALID_EXPENSE", "receiptReference cannot exceed 256 characters.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	old, err := q.GetExpenseForUpdate(ctx, postgresql.GetExpenseForUpdateParams{UserID: p.UserID, BusinessID: p.BusinessID, ExpenseID: id, IsAdmin: p.Role == "ADMIN"})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	if old.Version != expected {
		return conflict(c, "STALE_EXPENSE", "Expense was changed by another request; reload it and retry.")
	}
	if p.Role != "ADMIN" {
		if _, err := q.LockExpenseOutletAssignment(ctx, postgresql.LockExpenseOutletAssignmentParams{BusinessID: p.BusinessID, UserID: p.UserID, OutletID: old.OutletID}); errors.Is(err, pgx.ErrNoRows) {
			return notFound(c)
		} else if err != nil {
			return internalError(c)
		}
	}
	if _, err := q.GetActiveExpenseCategory(ctx, postgresql.GetActiveExpenseCategoryParams{BusinessID: p.BusinessID, CategoryID: *req.CategoryID}); errors.Is(err, pgx.ErrNoRows) {
		return conflict(c, "INACTIVE_EXPENSE_CATEGORY", "Expense category is unavailable or inactive.")
	} else if err != nil {
		return internalError(c)
	}
	item, err := q.UpdateExpense(ctx, postgresql.UpdateExpenseParams{CategoryID: *req.CategoryID, Amount: *req.Amount, Description: desc, ExpenseAt: pgtype.Timestamptz{Time: req.ExpenseAt.UTC(), Valid: true}, ReceiptReference: optionalDBText(req.ReceiptReference), BusinessID: p.BusinessID, ExpenseID: id, ExpectedVersion: expected, UserID: p.UserID, IsAdmin: p.Role == "ADMIN"})
	if errors.Is(err, pgx.ErrNoRows) {
		return conflict(c, "STALE_EXPENSE", "Expense or category is no longer available; reload and retry.")
	}
	if err != nil {
		return expenseWriteError(c, err)
	}
	before, after := expenseFrom(old), expenseFrom(item)
	if err := writeExpenseAudit(ctx, q, p, c, "EXPENSE_UPDATED", id, after.OutletID, expenseAuditValues(before), expenseAuditValues(after)); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	c.Response().Header().Set("ETag", expenseETag(after.Version))
	return c.JSON(http.StatusOK, after)
}

func expenseCategoryFrom(i postgresql.ExpenseCategory) expenseCategoryResponse {
	return expenseCategoryResponse{ID: i.ID, BusinessID: i.BusinessID, Name: i.Name, Description: dbTextPtr(i.Description), IsActive: i.IsActive, CreatedAt: i.CreatedAt.Time, UpdatedAt: i.UpdatedAt.Time}
}
func expenseFrom(i postgresql.Expense) expenseResponse {
	return expenseResponse{ID: i.ID, BusinessID: i.BusinessID, OutletID: i.OutletID, CategoryID: i.CategoryID, Amount: i.Amount, Description: i.Description, ExpenseAt: i.ExpenseAt.Time, ReceiptReference: dbTextPtr(i.ReceiptReference), CreatedBy: i.CreatedBy, CreatedAt: i.CreatedAt.Time, UpdatedAt: i.UpdatedAt.Time, Version: i.Version}
}
func optionalDBText(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	s := strings.TrimSpace(*v)
	if s == "" {
		return pgtype.Text{}
	}
	return pgtype.Text{String: s, Valid: true}
}
func dbTextPtr(v pgtype.Text) *string {
	if !v.Valid {
		return nil
	}
	s := v.String
	return &s
}
func categoryAuditValues(v expenseCategoryResponse) map[string]any {
	return map[string]any{"id": v.ID, "name": v.Name, "is_active": v.IsActive}
}
func expenseAuditValues(v expenseResponse) map[string]any {
	return map[string]any{"id": v.ID, "outlet_id": v.OutletID, "category_id": v.CategoryID, "amount": v.Amount, "expense_at": v.ExpenseAt, "version": v.Version, "description": "[REDACTED]", "receipt_reference": "[REDACTED]"}
}
func writeExpenseAudit(ctx context.Context, q *postgresql.Queries, p Principal, c echo.Context, action string, id, outletID int64, before, after any) error {
	oldJSON, err := json.Marshal(before)
	if err != nil {
		return err
	}
	newJSON, err := json.Marshal(after)
	if err != nil {
		return err
	}
	var ip *netip.Addr
	if parsed, e := netip.ParseAddr(c.RealIP()); e == nil {
		ip = &parsed
	}
	ua := c.Request().UserAgent()
	if len(ua) > 512 {
		ua = ua[:512]
	}
	return q.InsertExpenseAuditLog(ctx, postgresql.InsertExpenseAuditLogParams{BusinessID: p.BusinessID, OutletID: pgtype.Int8{Int64: outletID, Valid: true}, ActorUserID: pgtype.Int8{Int64: p.UserID, Valid: true}, Action: action, EntityID: pgtype.Int8{Int64: id, Valid: true}, OldValues: oldJSON, NewValues: newJSON, IpAddress: ip, UserAgent: pgtype.Text{String: ua, Valid: ua != ""}})
}
func writeExpenseCategoryAudit(ctx context.Context, q *postgresql.Queries, p Principal, c echo.Context, action string, id int64, before, after any) error {
	oldJSON, err := json.Marshal(before)
	if err != nil {
		return err
	}
	newJSON, err := json.Marshal(after)
	if err != nil {
		return err
	}
	var ip *netip.Addr
	if parsed, e := netip.ParseAddr(c.RealIP()); e == nil {
		ip = &parsed
	}
	ua := c.Request().UserAgent()
	if len(ua) > 512 {
		ua = ua[:512]
	}
	return q.InsertExpenseCategoryAuditLog(ctx, postgresql.InsertExpenseCategoryAuditLogParams{BusinessID: p.BusinessID, ActorUserID: pgtype.Int8{Int64: p.UserID, Valid: true}, Action: action, EntityID: pgtype.Int8{Int64: id, Valid: true}, OldValues: oldJSON, NewValues: newJSON, IpAddress: ip, UserAgent: pgtype.Text{String: ua, Valid: ua != ""}})
}
func expenseWriteError(c echo.Context, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return conflict(c, "EXPENSE_CATEGORY_CONFLICT", "A category with this name already exists.")
	}
	return internalError(c)
}

func expenseDateRange(c echo.Context) (time.Time, time.Time, error) {
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		return time.Time{}, time.Time{}, err
	}
	today := time.Now().In(loc).Format("2006-01-02")
	fromRaw, toRaw := c.QueryParam("fromDate"), c.QueryParam("toDate")
	if fromRaw == "" && toRaw == "" {
		fromRaw, toRaw = today, today
	} else if fromRaw == "" {
		fromRaw = toRaw
	} else if toRaw == "" {
		toRaw = fromRaw
	}
	from, err := time.ParseInLocation("2006-01-02", fromRaw, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("fromDate and toDate must use YYYY-MM-DD")
	}
	to, err := time.ParseInLocation("2006-01-02", toRaw, loc)
	if err != nil {
		return time.Time{}, time.Time{}, fmt.Errorf("fromDate and toDate must use YYYY-MM-DD")
	}
	if to.Before(from) {
		return time.Time{}, time.Time{}, fmt.Errorf("toDate must be on or after fromDate")
	}
	return from, to, nil
}
func dateParam(t time.Time) pgtype.Date {
	return pgtype.Date{Time: time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC), Valid: true}
}
func expenseETag(version int64) string { return fmt.Sprintf("\"%d\"", version) }
func parseExpenseETag(value string) (int64, error) {
	value = strings.TrimSpace(value)
	if len(value) < 3 || value[0] != '"' || value[len(value)-1] != '"' {
		return 0, errors.New("invalid If-Match")
	}
	v, err := strconv.ParseInt(value[1:len(value)-1], 10, 64)
	if err != nil || v < 1 {
		return 0, errors.New("invalid If-Match")
	}
	return v, nil
}
