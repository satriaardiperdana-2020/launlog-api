package middleware

import (
	"net/http"
	"strconv"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/satriaardiperdana-2020/launlog-api/internal/handlers"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
)

func AuthenticatePlatform(db *repository.Postgres, tokens *security.PlatformTokenManager, forLogout bool) echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			parts := strings.Fields(c.Request().Header.Get(echo.HeaderAuthorization))
			if len(parts) != 2 || parts[0] != "Bearer" {
				return unauthorized(c)
			}
			claims, err := tokens.ParseAccessToken(parts[1])
			if err != nil {
				return unauthorized(c)
			}
			id, err := strconv.ParseInt(claims.Subject, 10, 64)
			if err != nil || id != 1 {
				return unauthorized(c)
			}
			q := db.Queries()
			if forLogout {
				if _, err := q.GetPlatformLogoutSession(c.Request().Context(), postgresql.GetPlatformLogoutSessionParams{ID: claims.SessionID, PlatformAdminID: id}); err != nil {
					return unauthorized(c)
				}
			} else {
				if _, err := q.GetActivePlatformSession(c.Request().Context(), postgresql.GetActivePlatformSessionParams{ID: claims.SessionID, PlatformAdminID: id}); err != nil {
					return unauthorized(c)
				}
			}
			admin, err := q.GetActivePlatformAdmin(c.Request().Context(), id)
			if err != nil {
				return unauthorized(c)
			}
			handlers.SetPlatformPrincipal(c, handlers.PlatformPrincipal{ID: id, SessionID: claims.SessionID, Email: admin.Email})
			return next(c)
		}
	}
}

// RequirePlatform protects future privileged platform routes even if middleware
// registration is accidentally omitted at an individual route.
func RequirePlatform() echo.MiddlewareFunc {
	return func(next echo.HandlerFunc) echo.HandlerFunc {
		return func(c echo.Context) error {
			if _, ok := handlers.PlatformPrincipalFromContext(c); !ok {
				return c.JSON(http.StatusUnauthorized, map[string]string{"code": "UNAUTHORIZED", "message": "Authentication is required."})
			}
			return next(c)
		}
	}
}
