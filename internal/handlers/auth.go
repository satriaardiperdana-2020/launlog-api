package handlers

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/labstack/echo/v4"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
)

type AuthHandler struct {
	database   *repository.Postgres
	tokens     *security.TokenManager
	refreshTTL time.Duration
}

func NewAuthHandler(d *repository.Postgres, t *security.TokenManager, refreshTTL time.Duration) *AuthHandler {
	return &AuthHandler{d, t, refreshTTL}
}

type loginRequest struct {
	Email    string `json:"email"`
	Password string `json:"password"`
}
type refreshRequest struct {
	RefreshToken string `json:"refreshToken"`
}

func decodeAuthBody(c echo.Context, dst any) error {
	decoder := json.NewDecoder(c.Request().Body)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(dst); err != nil {
		return err
	}
	var extra any
	if err := decoder.Decode(&extra); err != io.EOF {
		return errors.New("unexpected content after JSON object")
	}
	return nil
}

func authBodyError(c echo.Context, err error, message string) error {
	var sizeError *http.MaxBytesError
	if errors.As(err, &sizeError) {
		return c.JSON(http.StatusRequestEntityTooLarge, map[string]string{"code": "REQUEST_TOO_LARGE", "message": "Request body is too large."})
	}
	return badRequest(c, "INVALID_REQUEST", message)
}

func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := decodeAuthBody(c, &req); err != nil {
		return authBodyError(c, err, "Email and password are required.")
	}
	if strings.TrimSpace(req.Email) == "" || req.Password == "" {
		return badRequest(c, "INVALID_REQUEST", "Email and password are required.")
	}
	ctx := c.Request().Context()
	u, err := h.database.Queries().GetUserForLogin(ctx, strings.TrimSpace(req.Email))
	if errors.Is(err, pgx.ErrNoRows) {
		security.VerifyDummyPassword(req.Password)
		return unauthorized(c)
	}
	if err != nil {
		return internalError(c)
	}
	if len([]byte(req.Password)) > security.MaxBcryptPasswordBytes {
		security.VerifyDummyPassword(req.Password)
		return unauthorized(c)
	}
	passwordErr := security.VerifyPassword(u.PasswordHash, req.Password)
	if passwordErr != nil || !u.IsActive {
		return unauthorized(c)
	}
	return h.issue(c, u.ID, u.BusinessID, u.Email, u.FullName, u.Role)
}

func (h *AuthHandler) Refresh(c echo.Context) error {
	var req refreshRequest
	if err := decodeAuthBody(c, &req); err != nil {
		return authBodyError(c, err, "Refresh token is required.")
	}
	if req.RefreshToken == "" {
		return badRequest(c, "INVALID_REQUEST", "Refresh token is required.")
	}
	ctx := c.Request().Context()
	hash := security.HashRefreshToken(req.RefreshToken)
	lookup, err := h.database.Queries().FindRefreshTokenFamily(ctx, hash)
	if errors.Is(err, pgx.ErrNoRows) {
		return unauthorized(c)
	}
	if err != nil {
		return internalError(c)
	}
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	family, err := q.LockSessionFamily(ctx, postgresql.LockSessionFamilyParams{ID: lookup.FamilyID, BusinessID: lookup.BusinessID, UserID: lookup.UserID})
	if err != nil || family.RevokedAt.Valid {
		return unauthorized(c)
	}
	session, err := q.GetRefreshTokenForUpdate(ctx, hash)
	if err != nil || session.FamilyID != family.ID || session.UserID != family.UserID {
		return unauthorized(c)
	}
	if session.RevokedAt.Valid {
		// A replay invalidates the entire lineage; this transaction must commit.
		// Finish containment even if the replaying client disconnects.
		commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if err := q.RevokeSessionFamily(commitCtx, postgresql.RevokeSessionFamilyParams{ID: family.ID, BusinessID: family.BusinessID, UserID: family.UserID}); err != nil {
			return internalError(c)
		}
		if err := tx.Commit(commitCtx); err != nil {
			return internalError(c)
		}
		return unauthorized(c)
	}
	if !session.ExpiresAt.Valid || !session.ExpiresAt.Time.After(time.Now()) {
		return unauthorized(c)
	}
	u, err := q.GetActiveUser(ctx, postgresql.GetActiveUserParams{ID: family.UserID, BusinessID: family.BusinessID})
	if err != nil {
		return unauthorized(c)
	}
	outs, perms, err := h.authorization(ctx, q, u.BusinessID, u.ID, u.Role)
	if err != nil {
		return unauthorized(c)
	}
	raw, tokenHash, err := security.NewRefreshToken()
	if err != nil {
		return internalError(c)
	}
	newSession, err := q.CreateRefreshToken(ctx, postgresql.CreateRefreshTokenParams{
		BusinessID: u.BusinessID, UserID: u.ID, FamilyID: family.ID,
		TokenHash: tokenHash, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(h.refreshTTL), Valid: true},
	})
	if err != nil {
		return internalError(c)
	}
	response, err := h.sessionResponse(u.ID, u.BusinessID, u.Email, u.FullName, u.Role, newSession.ID, raw, newSession.ExpiresAt.Time, outs, perms)
	if err != nil {
		return internalError(c)
	}
	affected, err := q.RevokeRefreshToken(ctx, postgresql.RevokeRefreshTokenParams{ID: session.ID, BusinessID: session.BusinessID, UserID: session.UserID})
	if err != nil || affected != 1 {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) Logout(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	var req refreshRequest
	if err := decodeAuthBody(c, &req); err != nil {
		return authBodyError(c, err, "Refresh token is required.")
	}
	if req.RefreshToken == "" {
		return badRequest(c, "INVALID_REQUEST", "Refresh token is required.")
	}
	ctx := c.Request().Context()
	hash := security.HashRefreshToken(req.RefreshToken)
	lookup, err := h.database.Queries().FindRefreshTokenFamily(ctx, hash)
	if err != nil || lookup.BusinessID != p.BusinessID || lookup.UserID != p.UserID {
		return unauthorized(c)
	}
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	family, err := q.LockSessionFamily(ctx, postgresql.LockSessionFamilyParams{ID: lookup.FamilyID, BusinessID: p.BusinessID, UserID: p.UserID})
	if err != nil || family.RevokedAt.Valid {
		return unauthorized(c)
	}
	// The bearer can be consumed if refresh won the race with logout.
	bearer, err := q.GetRefreshTokenByID(ctx, postgresql.GetRefreshTokenByIDParams{ID: p.SessionID, BusinessID: p.BusinessID, UserID: p.UserID})
	if err != nil || bearer.FamilyID != family.ID {
		return unauthorized(c)
	}
	if err := q.RevokeSessionFamily(ctx, postgresql.RevokeSessionFamilyParams{ID: family.ID, BusinessID: family.BusinessID, UserID: family.UserID}); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *AuthHandler) Me(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	return c.JSON(http.StatusOK, map[string]any{"id": p.UserID, "businessId": p.BusinessID, "email": p.Email, "fullName": p.FullName, "role": p.Role, "outletIds": p.OutletIDs, "permissions": p.Permissions})
}

func (h *AuthHandler) issue(c echo.Context, id, bid int64, email, name, role string) error {
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	outs, perms, err := h.authorization(ctx, q, bid, id, role)
	if err != nil {
		return unauthorized(c)
	}
	familyID, err := q.CreateSessionFamily(ctx, postgresql.CreateSessionFamilyParams{BusinessID: bid, UserID: id})
	if err != nil {
		return internalError(c)
	}
	raw, hash, err := security.NewRefreshToken()
	if err != nil {
		return internalError(c)
	}
	session, err := q.CreateRefreshToken(ctx, postgresql.CreateRefreshTokenParams{
		BusinessID: bid, UserID: id, FamilyID: familyID, TokenHash: hash,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(h.refreshTTL), Valid: true},
	})
	if err != nil {
		return internalError(c)
	}
	response, err := h.sessionResponse(id, bid, email, name, role, session.ID, raw, session.ExpiresAt.Time, outs, perms)
	if err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, response)
}

func (h *AuthHandler) authorization(ctx context.Context, q *postgresql.Queries, bid, uid int64, role string) ([]int64, []string, error) {
	outs, err := q.ListUserOutletIDs(ctx, postgresql.ListUserOutletIDsParams{BusinessID: bid, UserID: uid})
	if err != nil {
		return nil, nil, err
	}
	if role == "LAUNDRY_STAFF" && len(outs) == 0 {
		return nil, nil, errors.New("staff has no active outlet")
	}
	perms, err := q.ListUserPermissionCodes(ctx, postgresql.ListUserPermissionCodesParams{BusinessID: bid, UserID: uid})
	return outs, perms, err
}

func (h *AuthHandler) sessionResponse(id, bid int64, email, name, role string, sid int64, refresh string, refreshExpiry time.Time, outs []int64, perms []string) (map[string]any, error) {
	access, accessExp, err := h.tokens.NewAccessToken(id, bid, sid, role, outs, time.Now())
	if err != nil {
		return nil, err
	}
	return map[string]any{"tokens": map[string]any{"accessToken": access, "refreshToken": refresh, "tokenType": "Bearer", "accessTokenExpiresAt": accessExp, "refreshTokenExpiresAt": refreshExpiry}, "user": map[string]any{"id": id, "businessId": bid, "email": email, "fullName": name, "role": role, "outletIds": outs, "permissions": perms}}, nil
}

func badRequest(c echo.Context, code, msg string) error {
	return c.JSON(http.StatusBadRequest, map[string]string{"code": code, "message": msg})
}
func unauthorized(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderWWWAuthenticate, "Bearer")
	return c.JSON(http.StatusUnauthorized, map[string]string{"code": "UNAUTHORIZED", "message": "Authentication is required."})
}
func internalError(c echo.Context) error {
	return c.JSON(http.StatusInternalServerError, map[string]string{"code": "INTERNAL_ERROR", "message": "An unexpected error occurred."})
}
