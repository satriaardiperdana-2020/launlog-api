package handlers

import (
	"net/http"
	"strings"
	"time"

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

func (h *AuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := c.Bind(&req); err != nil || strings.TrimSpace(req.Email) == "" || req.Password == "" {
		return badRequest(c, "INVALID_REQUEST", "Email and password are required.")
	}
	u, err := h.database.Queries().GetUserForLogin(c.Request().Context(), strings.TrimSpace(req.Email))
	if err != nil || !u.IsActive || security.VerifyPassword(u.PasswordHash, req.Password) != nil {
		return unauthorized(c)
	}
	return h.issue(c, u.ID, u.BusinessID, u.Email, u.FullName, u.Role)
}
func (h *AuthHandler) Refresh(c echo.Context) error {
	var req refreshRequest
	if err := c.Bind(&req); err != nil || req.RefreshToken == "" {
		return badRequest(c, "INVALID_REQUEST", "Refresh token is required.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	session, err := q.GetRefreshTokenForUpdate(ctx, security.HashRefreshToken(req.RefreshToken))
	if err != nil || session.RevokedAt.Valid || !session.ExpiresAt.Valid || !session.ExpiresAt.Time.After(time.Now()) {
		return unauthorized(c)
	}
	u, err := q.GetActiveUser(ctx, postgresql.GetActiveUserParams{ID: session.UserID, BusinessID: session.BusinessID})
	if err != nil {
		return unauthorized(c)
	}
	if _, err = q.RevokeRefreshToken(ctx, postgresql.RevokeRefreshTokenParams{ID: session.ID, BusinessID: session.BusinessID, UserID: session.UserID}); err != nil {
		return internalError(c)
	}
	raw, hash, err := security.NewRefreshToken()
	if err != nil {
		return internalError(c)
	}
	newSession, err := q.CreateRefreshToken(ctx, postgresql.CreateRefreshTokenParams{BusinessID: u.BusinessID, UserID: u.ID, TokenHash: hash, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(h.refreshTTL), Valid: true}})
	if err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return h.respondSession(c, u.ID, u.BusinessID, u.Email, u.FullName, u.Role, newSession.ID, raw)
}
func (h *AuthHandler) Logout(c echo.Context) error {
	p, ok := PrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	var req refreshRequest
	if err := c.Bind(&req); err != nil || req.RefreshToken == "" {
		return badRequest(c, "INVALID_REQUEST", "Refresh token is required.")
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	s, err := q.GetRefreshTokenForUpdate(ctx, security.HashRefreshToken(req.RefreshToken))
	if err != nil || s.ID != p.SessionID || s.UserID != p.UserID || s.BusinessID != p.BusinessID {
		return unauthorized(c)
	}
	if _, err = q.RevokeRefreshToken(ctx, postgresql.RevokeRefreshTokenParams{ID: s.ID, BusinessID: s.BusinessID, UserID: s.UserID}); err != nil {
		return internalError(c)
	}
	if err = tx.Commit(ctx); err != nil {
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
	raw, hash, err := security.NewRefreshToken()
	if err != nil {
		return internalError(c)
	}
	s, err := h.database.Queries().CreateRefreshToken(c.Request().Context(), postgresql.CreateRefreshTokenParams{BusinessID: bid, UserID: id, TokenHash: hash, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(h.refreshTTL), Valid: true}})
	if err != nil {
		return internalError(c)
	}
	return h.respondSession(c, id, bid, email, name, role, s.ID, raw)
}
func (h *AuthHandler) respondSession(c echo.Context, id, bid int64, email, name, role string, sid int64, refresh string) error {
	q := h.database.Queries()
	outs, err := q.ListUserOutletIDs(c.Request().Context(), postgresql.ListUserOutletIDsParams{BusinessID: bid, UserID: id})
	if err != nil {
		return internalError(c)
	}
	perms, err := q.ListUserPermissionCodes(c.Request().Context(), postgresql.ListUserPermissionCodesParams{BusinessID: bid, UserID: id})
	if err != nil {
		return internalError(c)
	}
	access, accessExp, err := h.tokens.NewAccessToken(id, bid, sid, role, outs, time.Now())
	if err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, map[string]any{"tokens": map[string]any{"accessToken": access, "refreshToken": refresh, "tokenType": "Bearer", "accessTokenExpiresAt": accessExp, "refreshTokenExpiresAt": time.Now().Add(h.refreshTTL)}, "user": map[string]any{"id": id, "businessId": bid, "email": email, "fullName": name, "role": role, "outletIds": outs, "permissions": perms}})
}
func badRequest(c echo.Context, code, msg string) error {
	return c.JSON(400, map[string]string{"code": code, "message": msg})
}
func unauthorized(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderWWWAuthenticate, "Bearer")
	return c.JSON(401, map[string]string{"code": "UNAUTHORIZED", "message": "Authentication is required."})
}
func internalError(c echo.Context) error {
	return c.JSON(500, map[string]string{"code": "INTERNAL_ERROR", "message": "An unexpected error occurred."})
}
