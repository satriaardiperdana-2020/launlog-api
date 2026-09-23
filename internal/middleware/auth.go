package middleware

import (
	"net/http"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/satriaardiperdana-2020/launlog-api/internal/handlers"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
)

func Authenticate(db *repository.Postgres, tokens *security.TokenManager) echo.MiddlewareFunc {
	return authenticate(db, tokens, false)
}

// AuthenticateForLogout permits an already consumed bearer session so logout
// can revoke its family if refresh committed just before logout began.
func AuthenticateForLogout(db *repository.Postgres, tokens *security.TokenManager) echo.MiddlewareFunc {
	return authenticate(db, tokens, true)
}

func authenticate(db *repository.Postgres, tokens *security.TokenManager, forLogout bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			h := c.Request().Header.Get(echo.HeaderAuthorization)
			parts := strings.Fields(h)
			if len(parts) != 2 || parts[0] != "Bearer" {
				return unauthorized(c)
			}
			claims, err := tokens.ParseAccessToken(parts[1])
			if err != nil {
				return unauthorized(c)
			}
			uid, err := security.SubjectID(claims)
			if err != nil {
				return unauthorized(c)
			}
			q := db.Queries()
			if forLogout {
				if _, err = q.GetLogoutSession(c.Request().Context(), postgresql.GetLogoutSessionParams{ID: claims.SessionID, BusinessID: claims.BusinessID, UserID: uid}); err != nil {
					return unauthorized(c)
				}
			} else {
				if _, err = q.GetActiveRefreshSession(c.Request().Context(), postgresql.GetActiveRefreshSessionParams{ID: claims.SessionID, BusinessID: claims.BusinessID, UserID: uid}); err != nil {
					return unauthorized(c)
				}
			}
			u, err := q.GetActiveUser(c.Request().Context(), postgresql.GetActiveUserParams{ID: uid, BusinessID: claims.BusinessID})
			if err != nil {
				return unauthorized(c)
			}
			if u.Role != claims.Role {
				return unauthorized(c)
			}
			outs, err := q.ListUserOutletIDs(c.Request().Context(), postgresql.ListUserOutletIDsParams{BusinessID: claims.BusinessID, UserID: uid})
			if err != nil {
				return unauthorized(c)
			}
			if !same(outs, claims.OutletIDs) {
				return unauthorized(c)
			}
			if u.Role == "LAUNDRY_STAFF" && len(outs) == 0 {
				return unauthorized(c)
			}
			perms, err := q.ListUserPermissionCodes(c.Request().Context(), postgresql.ListUserPermissionCodesParams{BusinessID: claims.BusinessID, UserID: uid})
			if err != nil {
				return unauthorized(c)
			}
			handlers.SetPrincipal(c, handlers.Principal{UserID: uid, BusinessID: claims.BusinessID, SessionID: claims.SessionID, Email: u.Email, FullName: u.FullName, Role: u.Role, OutletIDs: outs, Permissions: perms})
			return next(c)
		}
	}
}
func RequirePermission(permission string) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p, ok := handlers.PrincipalFromContext(c)
			if !ok {
				return unauthorized(c)
			}
			if p.Role == "ADMIN" {
				return next(c)
			}
			for _, v := range p.Permissions {
				if v == permission {
					return next(c)
				}
			}
			return c.JSON(http.StatusForbidden, map[string]string{"code": "FORBIDDEN", "message": "Permission is required."})
		}
	}
}

// RequireAdmin limits owner-management operations to ADMIN while relying on
// Authenticate to validate the active tenant and current outlet assignments.
func RequireAdmin() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			p, ok := handlers.PrincipalFromContext(c)
			if !ok {
				return unauthorized(c)
			}
			if p.Role != "ADMIN" {
				return c.JSON(http.StatusForbidden, map[string]string{"code": "FORBIDDEN", "message": "Administrator access is required."})
			}
			return next(c)
		}
	}
}
func unauthorized(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderWWWAuthenticate, "Bearer")
	return c.JSON(http.StatusUnauthorized, map[string]string{"code": "UNAUTHORIZED", "message": "Authentication is required."})
}
func same(a, b []int64) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
