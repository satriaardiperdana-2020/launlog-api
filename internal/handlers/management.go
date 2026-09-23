package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/mail"
	"net/netip"
	"strconv"
	"strings"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
)

type ManagementHandler struct{ database *repository.Postgres }

func NewManagementHandler(database *repository.Postgres) *ManagementHandler {
	return &ManagementHandler{database: database}
}

type outletInput struct {
	Code     string  `json:"code"`
	Name     string  `json:"name"`
	Phone    *string `json:"phone"`
	Address  *string `json:"address"`
	IsActive *bool   `json:"isActive"`
}

type createStaffInput struct {
	Email     string  `json:"email"`
	FullName  string  `json:"fullName"`
	Password  string  `json:"password"`
	OutletIDs []int64 `json:"outletIds"`
}

type updateStaffInput struct {
	FullName string `json:"fullName"`
	IsActive *bool  `json:"isActive"`
}

type outletAssignmentInput struct {
	OutletIDs []int64 `json:"outletIds"`
}
type permissionGrantInput struct {
	Permissions []string `json:"permissions"`
}

func decodeManagementJSON(c echo.Context, dst any) error {
	d := json.NewDecoder(c.Request().Body)
	d.DisallowUnknownFields()
	if err := d.Decode(dst); err != nil {
		return err
	}
	var trailing any
	if err := d.Decode(&trailing); errors.Is(err, io.EOF) {
		return nil
	}
	return errors.New("invalid trailing JSON")
}

func pagination(c echo.Context) (int32, int32, int32, error) {
	page, size := int64(1), int64(20)
	var err error
	if raw := c.QueryParam("page"); raw != "" {
		page, err = strconv.ParseInt(raw, 10, 32)
		if err != nil || page < 1 {
			return 0, 0, 0, errors.New("page must be a positive integer")
		}
	}
	if raw := c.QueryParam("pageSize"); raw != "" {
		size, err = strconv.ParseInt(raw, 10, 32)
		if err != nil || size < 1 || size > 100 {
			return 0, 0, 0, errors.New("pageSize must be between 1 and 100")
		}
	}
	offset := (page - 1) * size
	if offset > int64(^uint32(0)>>1) {
		return 0, 0, 0, errors.New("page offset is too large")
	}
	return int32(page), int32(size), int32(offset), nil
}

func (h *ManagementHandler) ListOutlets(c echo.Context) error {
	p, _ := PrincipalFromContext(c)
	page, size, offset, err := pagination(c)
	if err != nil {
		return badRequest(c, "INVALID_PAGINATION", err.Error())
	}
	ctx := c.Request().Context()
	q := h.database.Queries()
	items, err := q.ListOutlets(ctx, postgresql.ListOutletsParams{BusinessID: p.BusinessID, Limit: size, Offset: offset})
	if err != nil {
		return internalError(c)
	}
	total, err := q.CountOutlets(ctx, p.BusinessID)
	if err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items, "pagination": pageInfo(page, size, total)})
}

func (h *ManagementHandler) GetOutlet(c echo.Context) error {
	p, _ := PrincipalFromContext(c)
	id, err := pathID(c, "outletId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Outlet ID must be a positive integer.")
	}
	item, err := h.database.Queries().GetOutlet(c.Request().Context(), postgresql.GetOutletParams{BusinessID: p.BusinessID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, item)
}

func (h *ManagementHandler) CreateOutlet(c echo.Context) error {
	p, _ := PrincipalFromContext(c)
	var req outletInput
	if err := decodeManagementJSON(c, &req); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Outlet body is invalid.")
	}
	if !validOutlet(req) {
		return badRequest(c, "INVALID_REQUEST", "Outlet fields exceed allowed lengths or required values are empty.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	if err := lockOwnerContext(ctx, q, p); err != nil {
		return unauthorized(c)
	}
	item, err := q.CreateOutlet(ctx, postgresql.CreateOutletParams{BusinessID: p.BusinessID, Code: strings.TrimSpace(req.Code), Name: strings.TrimSpace(req.Name), Phone: textArg(req.Phone), Address: textArg(req.Address)})
	if err != nil {
		return dbWriteError(c, err)
	}
	if err := writeAudit(ctx, q, p, c, "OUTLET_CREATED", "outlet", item.ID, nil, map[string]any{"id": item.ID, "code": item.Code, "name": item.Name, "isActive": item.IsActive}); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusCreated, item)
}

func (h *ManagementHandler) UpdateOutlet(c echo.Context) error {
	p, _ := PrincipalFromContext(c)
	id, err := pathID(c, "outletId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "Outlet ID must be a positive integer.")
	}
	var req outletInput
	if err := decodeManagementJSON(c, &req); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Outlet body is invalid.")
	}
	if !validOutlet(req) || req.IsActive == nil {
		return badRequest(c, "INVALID_REQUEST", "Outlet code, name, and isActive are required and fields must fit allowed lengths.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	if err := lockOwnerContext(ctx, q, p); err != nil {
		return unauthorized(c)
	}
	old, err := q.GetOutletForUpdate(ctx, postgresql.GetOutletForUpdateParams{BusinessID: p.BusinessID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	if old.IsActive && !*req.IsActive {
		count, err := q.CountActiveAssignedStaffWithoutOtherOutlet(ctx, postgresql.CountActiveAssignedStaffWithoutOtherOutletParams{BusinessID: p.BusinessID, OutletID: id})
		if err != nil {
			return internalError(c)
		}
		if count > 0 {
			return conflict(c, "OUTLET_HAS_ASSIGNED_STAFF", "Reassign active staff before deactivating their only outlet.")
		}
	}
	item, err := q.UpdateOutlet(ctx, postgresql.UpdateOutletParams{BusinessID: p.BusinessID, ID: id, Code: strings.TrimSpace(req.Code), Name: strings.TrimSpace(req.Name), Phone: textArg(req.Phone), Address: textArg(req.Address), IsActive: *req.IsActive})
	if err != nil {
		return dbWriteError(c, err)
	}
	if err := writeAudit(ctx, q, p, c, "OUTLET_UPDATED", "outlet", id, map[string]any{"code": old.Code, "name": old.Name, "isActive": old.IsActive}, map[string]any{"code": item.Code, "name": item.Name, "isActive": item.IsActive}); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, item)
}

func (h *ManagementHandler) ListStaff(c echo.Context) error {
	p, _ := PrincipalFromContext(c)
	page, size, offset, err := pagination(c)
	if err != nil {
		return badRequest(c, "INVALID_PAGINATION", err.Error())
	}
	ctx := c.Request().Context()
	q := h.database.Queries()
	items, err := q.ListStaff(ctx, postgresql.ListStaffParams{BusinessID: p.BusinessID, Limit: size, Offset: offset})
	if err != nil {
		return internalError(c)
	}
	total, err := q.CountStaff(ctx, p.BusinessID)
	if err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items, "pagination": pageInfo(page, size, total)})
}

func (h *ManagementHandler) GetStaff(c echo.Context) error {
	p, _ := PrincipalFromContext(c)
	id, err := pathID(c, "userId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "User ID must be a positive integer.")
	}
	ctx := c.Request().Context()
	q := h.database.Queries()
	u, err := q.GetStaff(ctx, postgresql.GetStaffParams{BusinessID: p.BusinessID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	outs, err := q.ListStaffOutletIDs(ctx, postgresql.ListStaffOutletIDsParams{BusinessID: p.BusinessID, UserID: id})
	if err != nil {
		return internalError(c)
	}
	perms, err := q.ListStaffPermissionCodes(ctx, postgresql.ListStaffPermissionCodesParams{BusinessID: p.BusinessID, UserID: id})
	if err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, map[string]any{"id": u.ID, "businessId": u.BusinessID, "email": u.Email, "fullName": u.FullName, "role": u.Role, "isActive": u.IsActive, "outletIds": outs, "permissions": perms})
}

func (h *ManagementHandler) CreateStaff(c echo.Context) error {
	p, _ := PrincipalFromContext(c)
	var req createStaffInput
	if err := decodeManagementJSON(c, &req); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Staff body is invalid.")
	}
	req.Email = strings.TrimSpace(strings.ToLower(req.Email))
	req.FullName = strings.TrimSpace(req.FullName)
	parsedEmail, emailErr := mail.ParseAddress(req.Email)
	if emailErr != nil || parsedEmail.Address != req.Email || len(req.Email) > 254 || req.FullName == "" || len([]rune(req.FullName)) > 200 || len(req.OutletIDs) == 0 {
		return badRequest(c, "INVALID_REQUEST", "A valid email, fullName, password, and at least one active outlet are required.")
	}
	hash, err := security.HashPassword(req.Password)
	if err != nil {
		return badRequest(c, "INVALID_PASSWORD", "Password must meet the creation policy and bcrypt byte limit.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	if err := lockOwnerContext(ctx, q, p); err != nil {
		return unauthorized(c)
	}
	if err := validateOutletIDs(ctx, q, p.BusinessID, req.OutletIDs); err != nil {
		return badRequest(c, "INVALID_OUTLETS", "Every outlet must be active and belong to this business.")
	}
	u, err := q.CreateStaff(ctx, postgresql.CreateStaffParams{BusinessID: p.BusinessID, Email: req.Email, FullName: req.FullName, PasswordHash: hash})
	if err != nil {
		return dbWriteError(c, err)
	}
	for _, outletID := range uniqueIDs(req.OutletIDs) {
		if err := q.AddStaffOutlet(ctx, postgresql.AddStaffOutletParams{BusinessID: p.BusinessID, UserID: u.ID, OutletID: outletID}); err != nil {
			return internalError(c)
		}
	}
	if err := writeAudit(ctx, q, p, c, "STAFF_CREATED", "user", u.ID, nil, map[string]any{"id": u.ID, "email": u.Email, "fullName": u.FullName, "role": u.Role, "isActive": u.IsActive, "outletIds": uniqueIDs(req.OutletIDs)}); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusCreated, map[string]any{"id": u.ID, "businessId": u.BusinessID, "email": u.Email, "fullName": u.FullName, "role": u.Role, "isActive": u.IsActive, "outletIds": uniqueIDs(req.OutletIDs), "permissions": []string{}})
}

func (h *ManagementHandler) UpdateStaff(c echo.Context) error {
	p, _ := PrincipalFromContext(c)
	id, err := pathID(c, "userId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "User ID must be a positive integer.")
	}
	var req updateStaffInput
	if err := decodeManagementJSON(c, &req); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Staff body is invalid.")
	}
	req.FullName = strings.TrimSpace(req.FullName)
	if req.FullName == "" || len([]rune(req.FullName)) > 200 || req.IsActive == nil {
		return badRequest(c, "INVALID_REQUEST", "fullName must contain at most 200 characters and isActive is required.")
	}
	if id == p.UserID && !*req.IsActive {
		return badRequest(c, "SELF_DEACTIVATION", "An administrator cannot deactivate their own account.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	if err := lockOwnerContext(ctx, q, p); err != nil {
		return unauthorized(c)
	}
	old, err := q.GetStaffForUpdate(ctx, postgresql.GetStaffForUpdateParams{BusinessID: p.BusinessID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	if old.Role == "ADMIN" && old.IsActive && !*req.IsActive {
		count, err := q.CountOtherActiveAdmins(ctx, postgresql.CountOtherActiveAdminsParams{BusinessID: p.BusinessID, ID: id})
		if err != nil {
			return internalError(c)
		}
		if count == 0 {
			return conflict(c, "LAST_ACTIVE_OWNER", "The last active administrator cannot be deactivated.")
		}
	}
	u, err := q.UpdateStaff(ctx, postgresql.UpdateStaffParams{BusinessID: p.BusinessID, ID: id, FullName: req.FullName, IsActive: *req.IsActive})
	if err != nil {
		return dbWriteError(c, err)
	}
	if err := writeAudit(ctx, q, p, c, "STAFF_UPDATED", "user", id, map[string]any{"fullName": old.FullName, "isActive": old.IsActive}, map[string]any{"fullName": u.FullName, "isActive": u.IsActive}); err != nil {
		return internalError(c)
	}
	outs, err := q.ListStaffOutletIDs(ctx, postgresql.ListStaffOutletIDsParams{BusinessID: p.BusinessID, UserID: id})
	if err != nil {
		return internalError(c)
	}
	perms, err := q.ListStaffPermissionCodes(ctx, postgresql.ListStaffPermissionCodesParams{BusinessID: p.BusinessID, UserID: id})
	if err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, map[string]any{"id": u.ID, "businessId": u.BusinessID, "email": u.Email, "fullName": u.FullName, "role": u.Role, "isActive": u.IsActive, "outletIds": outs, "permissions": perms})
}

func (h *ManagementHandler) ReplaceStaffOutlets(c echo.Context) error {
	p, _ := PrincipalFromContext(c)
	id, err := pathID(c, "userId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "User ID must be a positive integer.")
	}
	if id == p.UserID {
		return badRequest(c, "SELF_ESCALATION", "Administrators cannot change their own outlet assignment through staff management.")
	}
	var req outletAssignmentInput
	if err := decodeManagementJSON(c, &req); err != nil || len(req.OutletIDs) == 0 {
		return badRequest(c, "INVALID_REQUEST", "At least one active outlet is required.")
	}
	ids := uniqueIDs(req.OutletIDs)
	if len(ids) != len(req.OutletIDs) {
		return badRequest(c, "INVALID_OUTLETS", "Outlet IDs must be unique.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	if err := lockOwnerContext(ctx, q, p); err != nil {
		return unauthorized(c)
	}
	u, err := q.GetStaffForUpdate(ctx, postgresql.GetStaffForUpdateParams{BusinessID: p.BusinessID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	if u.Role != "LAUNDRY_STAFF" {
		return conflict(c, "STAFF_ONLY", "Outlet assignments are managed only for laundry staff.")
	}
	old, err := q.ListStaffOutletIDs(ctx, postgresql.ListStaffOutletIDsParams{BusinessID: p.BusinessID, UserID: id})
	if err != nil {
		return internalError(c)
	}
	if err := validateOutletIDs(ctx, q, p.BusinessID, ids); err != nil {
		return badRequest(c, "INVALID_OUTLETS", "Every outlet must be active and belong to this business.")
	}
	if err := q.DeleteStaffOutlets(ctx, postgresql.DeleteStaffOutletsParams{BusinessID: p.BusinessID, UserID: id}); err != nil {
		return internalError(c)
	}
	for _, oid := range ids {
		if err := q.AddStaffOutlet(ctx, postgresql.AddStaffOutletParams{BusinessID: p.BusinessID, UserID: id, OutletID: oid}); err != nil {
			return internalError(c)
		}
	}
	if err := writeAudit(ctx, q, p, c, "STAFF_OUTLETS_REPLACED", "user", id, map[string]any{"outletIds": old}, map[string]any{"outletIds": ids}); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, map[string]any{"userId": id, "outletIds": ids})
}

func (h *ManagementHandler) ListPermissions(c echo.Context) error {
	items, err := h.database.Queries().ListPermissions(c.Request().Context())
	if err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, map[string]any{"items": items})
}

func (h *ManagementHandler) ReplaceStaffPermissions(c echo.Context) error {
	p, _ := PrincipalFromContext(c)
	id, err := pathID(c, "userId")
	if err != nil {
		return badRequest(c, "INVALID_ID", "User ID must be a positive integer.")
	}
	if id == p.UserID {
		return badRequest(c, "SELF_ESCALATION", "Administrators cannot grant permissions to themselves.")
	}
	var req permissionGrantInput
	if err := decodeManagementJSON(c, &req); err != nil {
		return badRequest(c, "INVALID_REQUEST", "Permission body is invalid.")
	}
	codes := uniqueStrings(req.Permissions)
	if len(codes) != len(req.Permissions) {
		return badRequest(c, "INVALID_PERMISSIONS", "Permission codes must be unique.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	if err := lockOwnerContext(ctx, q, p); err != nil {
		return unauthorized(c)
	}
	u, err := q.GetStaffForUpdate(ctx, postgresql.GetStaffForUpdateParams{BusinessID: p.BusinessID, ID: id})
	if errors.Is(err, pgx.ErrNoRows) {
		return notFound(c)
	}
	if err != nil {
		return internalError(c)
	}
	if u.Role != "LAUNDRY_STAFF" {
		return conflict(c, "STAFF_ONLY", "Explicit action grants are assigned only to laundry staff.")
	}
	known, err := q.ListPermissions(ctx)
	if err != nil {
		return internalError(c)
	}
	allowed := make(map[string]bool, len(known))
	for _, item := range known {
		allowed[item.Code] = true
	}
	for _, code := range codes {
		if !allowed[code] {
			return badRequest(c, "INVALID_PERMISSIONS", "One or more permission codes are not in the catalog.")
		}
	}
	old, err := q.ListStaffPermissionCodes(ctx, postgresql.ListStaffPermissionCodesParams{BusinessID: p.BusinessID, UserID: id})
	if err != nil {
		return internalError(c)
	}
	if err := q.DeleteStaffPermissions(ctx, postgresql.DeleteStaffPermissionsParams{BusinessID: p.BusinessID, UserID: id}); err != nil {
		return internalError(c)
	}
	for _, code := range codes {
		if err := q.AddStaffPermission(ctx, postgresql.AddStaffPermissionParams{BusinessID: p.BusinessID, UserID: id, PermissionCode: code, GrantedBy: p.UserID}); err != nil {
			return internalError(c)
		}
	}
	if err := writeAudit(ctx, q, p, c, "STAFF_PERMISSIONS_REPLACED", "user", id, map[string]any{"permissions": old}, map[string]any{"permissions": codes}); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, map[string]any{"userId": id, "permissions": codes})
}

func validateOutletIDs(ctx context.Context, q *postgresql.Queries, businessID int64, ids []int64) error {
	unique := uniqueIDs(ids)
	if len(unique) != len(ids) || len(ids) == 0 {
		return errors.New("outlet IDs must be nonempty and unique")
	}
	locked, err := q.LockActiveOutlets(ctx, postgresql.LockActiveOutletsParams{BusinessID: businessID, Column2: ids})
	if err != nil {
		return err
	}
	if int64(len(locked)) != int64(len(ids)) {
		return errors.New("outlets are not active in tenant")
	}
	return nil
}
func lockOwnerContext(ctx context.Context, q *postgresql.Queries, p Principal) error {
	if _, err := q.LockActiveBusiness(ctx, p.BusinessID); err != nil {
		return err
	}
	u, err := q.GetActiveUser(ctx, postgresql.GetActiveUserParams{ID: p.UserID, BusinessID: p.BusinessID})
	if err != nil {
		return err
	}
	if u.Role != "ADMIN" {
		return errors.New("administrator is no longer active")
	}
	return nil
}
func validOutlet(in outletInput) bool {
	if strings.TrimSpace(in.Code) == "" || strings.TrimSpace(in.Name) == "" || len([]rune(in.Code)) > 64 || len([]rune(in.Name)) > 200 {
		return false
	}
	if in.Phone != nil && len([]rune(*in.Phone)) > 32 {
		return false
	}
	if in.Address != nil && len([]rune(*in.Address)) > 500 {
		return false
	}
	return true
}
func writeAudit(ctx context.Context, q *postgresql.Queries, p Principal, c echo.Context, action, entity string, id int64, oldValues, newValues any) error {
	var oldJSON, newJSON []byte
	var err error
	if oldValues != nil {
		oldJSON, err = json.Marshal(oldValues)
		if err != nil {
			return err
		}
	}
	if newValues != nil {
		newJSON, err = json.Marshal(newValues)
		if err != nil {
			return err
		}
	}
	var address *netip.Addr
	if parsed, err := netip.ParseAddr(c.RealIP()); err == nil {
		address = &parsed
	}
	ua := c.Request().UserAgent()
	if len(ua) > 512 {
		ua = ua[:512]
	}
	return q.InsertAuditLog(ctx, postgresql.InsertAuditLogParams{BusinessID: p.BusinessID, ActorUserID: pgtype.Int8{Int64: p.UserID, Valid: true}, Action: action, EntityType: entity, EntityID: pgtype.Int8{Int64: id, Valid: true}, OldValues: oldJSON, NewValues: newJSON, IpAddress: address, UserAgent: pgtype.Text{String: ua, Valid: ua != ""}})
}
func textArg(v *string) pgtype.Text {
	if v == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *v, Valid: true}
}
func pathID(c echo.Context, key string) (int64, error) {
	id, err := strconv.ParseInt(c.Param(key), 10, 64)
	if err != nil || id < 1 {
		return 0, fmt.Errorf("invalid id")
	}
	return id, nil
}
func uniqueIDs(input []int64) []int64 {
	seen := map[int64]bool{}
	out := make([]int64, 0, len(input))
	for _, id := range input {
		if id > 0 && !seen[id] {
			seen[id] = true
			out = append(out, id)
		}
	}
	return out
}
func uniqueStrings(input []string) []string {
	seen := map[string]bool{}
	out := make([]string, 0, len(input))
	for _, s := range input {
		s = strings.TrimSpace(s)
		if s != "" && !seen[s] {
			seen[s] = true
			out = append(out, s)
		}
	}
	return out
}
func pageInfo(page, size int32, total int64) map[string]any {
	pages := int64(0)
	if total > 0 {
		pages = (total + int64(size) - 1) / int64(size)
	}
	return map[string]any{"page": page, "pageSize": size, "totalItems": total, "totalPages": pages}
}
func notFound(c echo.Context) error {
	return c.JSON(http.StatusNotFound, map[string]string{"code": "NOT_FOUND", "message": "The requested resource was not found."})
}
func conflict(c echo.Context, code, message string) error {
	return c.JSON(http.StatusConflict, map[string]string{"code": code, "message": message})
}
func dbWriteError(c echo.Context, err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return conflict(c, "DUPLICATE_RESOURCE", "A resource with the same unique value already exists.")
	}
	return internalError(c)
}
