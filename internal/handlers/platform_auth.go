package handlers

import (
	"context"
	"errors"
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

const platformPrincipalKey = "auth.platform_principal"

type PlatformPrincipal struct {
	ID, SessionID int64
	Email         string
}

func PlatformPrincipalFromContext(c echo.Context) (PlatformPrincipal, bool) {
	p, ok := c.Get(platformPrincipalKey).(PlatformPrincipal)
	return p, ok
}

func SetPlatformPrincipal(c echo.Context, p PlatformPrincipal) { c.Set(platformPrincipalKey, p) }

type PlatformAuthHandler struct {
	database   *repository.Postgres
	tokens     *security.PlatformTokenManager
	refreshTTL time.Duration
}

func NewPlatformAuthHandler(d *repository.Postgres, t *security.PlatformTokenManager, refreshTTL time.Duration) *PlatformAuthHandler {
	return &PlatformAuthHandler{database: d, tokens: t, refreshTTL: refreshTTL}
}

func (h *PlatformAuthHandler) Login(c echo.Context) error {
	var req loginRequest
	if err := decodeAuthBody(c, &req); err != nil {
		return authBodyError(c, err, "Email and password are required.")
	}
	if strings.TrimSpace(req.Email) == "" || req.Password == "" {
		return badRequest(c, "INVALID_REQUEST", "Email and password are required.")
	}
	u, err := h.database.Queries().GetPlatformAdminForLogin(c.Request().Context(), req.Email)
	if errors.Is(err, pgx.ErrNoRows) {
		security.VerifyDummyPassword(req.Password)
		return platformLoginDenied(c, h.database.Queries())
	}
	if err != nil {
		return internalError(c)
	}
	if len([]byte(req.Password)) > security.MaxBcryptPasswordBytes {
		security.VerifyDummyPassword(req.Password)
		return platformLoginDenied(c, h.database.Queries())
	}
	if security.VerifyPassword(u.PasswordHash, req.Password) != nil || !u.IsActive {
		return platformLoginDenied(c, h.database.Queries())
	}
	ctx := c.Request().Context()
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	if _, err := q.GetActivePlatformAdmin(ctx, u.ID); err != nil {
		return unauthorized(c)
	}
	familyID, err := q.CreatePlatformSessionFamily(ctx, u.ID)
	if err != nil {
		return internalError(c)
	}
	response, err := h.createSession(ctx, q, u.ID, u.Email, familyID)
	if err != nil || platformAudit(ctx, q, u.ID, "PLATFORM_SESSION_CREATED", familyID) != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, response)
}

func (h *PlatformAuthHandler) Refresh(c echo.Context) error {
	var req refreshRequest
	if err := decodeAuthBody(c, &req); err != nil {
		return authBodyError(c, err, "Refresh token is required.")
	}
	if req.RefreshToken == "" {
		return badRequest(c, "INVALID_REQUEST", "Refresh token is required.")
	}
	ctx := c.Request().Context()
	hash := security.HashRefreshToken(req.RefreshToken)
	lookup, err := h.database.Queries().FindPlatformRefreshFamily(ctx, hash)
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
	family, err := q.LockPlatformSessionFamily(ctx, postgresql.LockPlatformSessionFamilyParams{ID: lookup.FamilyID, PlatformAdminID: lookup.PlatformAdminID})
	if err != nil || family.RevokedAt.Valid {
		return unauthorized(c)
	}
	session, err := q.GetPlatformRefreshForUpdate(ctx, hash)
	if err != nil || session.FamilyID != family.ID || session.PlatformAdminID != family.PlatformAdminID {
		return unauthorized(c)
	}
	if session.RevokedAt.Valid {
		commitCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), 5*time.Second)
		defer cancel()
		if affected, err := q.RevokePlatformSessionFamily(commitCtx, postgresql.RevokePlatformSessionFamilyParams{ID: family.ID, PlatformAdminID: family.PlatformAdminID}); err != nil || affected != 1 {
			return internalError(c)
		}
		if err := platformAudit(commitCtx, q, family.PlatformAdminID, "PLATFORM_REFRESH_REPLAY_DETECTED", family.ID); err != nil {
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
	u, err := q.GetActivePlatformAdmin(ctx, family.PlatformAdminID)
	if err != nil {
		return unauthorized(c)
	}
	response, err := h.createSession(ctx, q, u.ID, u.Email, family.ID)
	if err != nil {
		return internalError(c)
	}
	if affected, err := q.RevokePlatformRefreshToken(ctx, postgresql.RevokePlatformRefreshTokenParams{ID: session.ID, PlatformAdminID: u.ID}); err != nil || affected != 1 {
		return internalError(c)
	}
	if err := platformAudit(ctx, q, u.ID, "PLATFORM_SESSION_REFRESHED", family.ID); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.JSON(http.StatusOK, response)
}

func (h *PlatformAuthHandler) Logout(c echo.Context) error {
	p, ok := PlatformPrincipalFromContext(c)
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
	lookup, err := h.database.Queries().FindPlatformRefreshFamily(ctx, security.HashRefreshToken(req.RefreshToken))
	if err != nil || lookup.PlatformAdminID != p.ID {
		return unauthorized(c)
	}
	tx, err := h.database.Begin(ctx)
	if err != nil {
		return internalError(c)
	}
	defer tx.Rollback(ctx)
	q := h.database.Queries().WithTx(tx)
	family, err := q.LockPlatformSessionFamily(ctx, postgresql.LockPlatformSessionFamilyParams{ID: lookup.FamilyID, PlatformAdminID: p.ID})
	if err != nil || family.RevokedAt.Valid {
		return unauthorized(c)
	}
	bearer, err := q.GetPlatformRefreshByID(ctx, postgresql.GetPlatformRefreshByIDParams{ID: p.SessionID, PlatformAdminID: p.ID})
	if err != nil || bearer.FamilyID != family.ID {
		return unauthorized(c)
	}
	if affected, err := q.RevokePlatformSessionFamily(ctx, postgresql.RevokePlatformSessionFamilyParams{ID: family.ID, PlatformAdminID: p.ID}); err != nil || affected != 1 {
		return internalError(c)
	}
	if err := platformAudit(ctx, q, p.ID, "PLATFORM_SESSION_REVOKED", family.ID); err != nil {
		return internalError(c)
	}
	if err := tx.Commit(ctx); err != nil {
		return internalError(c)
	}
	return c.NoContent(http.StatusNoContent)
}

func (h *PlatformAuthHandler) Me(c echo.Context) error {
	p, ok := PlatformPrincipalFromContext(c)
	if !ok {
		return unauthorized(c)
	}
	return c.JSON(http.StatusOK, map[string]any{"id": p.ID, "email": p.Email, "role": "PLATFORM_ADMIN"})
}

func (h *PlatformAuthHandler) createSession(ctx context.Context, q *postgresql.Queries, id int64, email string, familyID int64) (map[string]any, error) {
	raw, hash, err := security.NewRefreshToken()
	if err != nil {
		return nil, err
	}
	session, err := q.CreatePlatformRefreshToken(ctx, postgresql.CreatePlatformRefreshTokenParams{
		PlatformAdminID: id, FamilyID: familyID, TokenHash: hash,
		ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(h.refreshTTL), Valid: true},
	})
	if err != nil {
		return nil, err
	}
	access, expiry, err := h.tokens.NewAccessToken(id, session.ID, time.Now())
	if err != nil {
		return nil, err
	}
	return map[string]any{"tokens": map[string]any{"accessToken": access, "refreshToken": raw, "tokenType": "Bearer", "accessTokenExpiresAt": expiry, "refreshTokenExpiresAt": session.ExpiresAt.Time}, "admin": map[string]any{"id": id, "email": email, "role": "PLATFORM_ADMIN"}}, nil
}

func platformAudit(ctx context.Context, q *postgresql.Queries, actorID int64, action string, familyID int64) error {
	return q.InsertPlatformAuditLog(ctx, postgresql.InsertPlatformAuditLogParams{
		ActorPlatformAdminID: pgtype.Int8{Int64: actorID, Valid: true},
		Action:               action, TargetType: "session_family", TargetID: pgtype.Int8{Int64: familyID, Valid: true},
		Outcome: "SUCCESS",
	})
}

// Failed credentials carry no supplied identifier or secret into audit data.
// The route's bounded direct-peer limiter limits the rate of these records.
func platformLoginDenied(c echo.Context, q *postgresql.Queries) error {
	if err := q.InsertPlatformAuditLog(c.Request().Context(), postgresql.InsertPlatformAuditLogParams{
		Action: "PLATFORM_LOGIN_DENIED", TargetType: "platform_auth", Outcome: "DENIED",
	}); err != nil {
		return internalError(c)
	}
	return unauthorized(c)
}
