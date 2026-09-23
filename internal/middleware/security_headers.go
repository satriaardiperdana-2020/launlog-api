package middleware

import (
	"github.com/labstack/echo/v4"
	echomiddleware "github.com/labstack/echo/v4/middleware"
)

// SecurityHeaders adds conservative headers appropriate for a JSON-only API.
// HSTS is intentionally left to the TLS-terminating edge, whose deployment
// policy can safely scope it to the public HTTPS host.
func SecurityHeaders(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		header := c.Response().Header()
		header.Set(echo.HeaderXContentTypeOptions, "nosniff")
		header.Set(echo.HeaderXFrameOptions, "DENY")
		header.Set(echo.HeaderReferrerPolicy, "no-referrer")
		header.Set(echo.HeaderContentSecurityPolicy, "default-src 'none'; frame-ancestors 'none'; base-uri 'none'")
		header.Set("Cross-Origin-Resource-Policy", "same-origin")
		header.Set("X-XSS-Protection", "0")
		return next(c)
	}
}

// NoStore is used for authentication responses and identity endpoints that
// contain bearer credentials or personal account data.
func NoStore(next echo.HandlerFunc) echo.HandlerFunc {
	return func(c echo.Context) error {
		c.Response().Header().Set(echo.HeaderCacheControl, "no-store")
		c.Response().Header().Set("Pragma", "no-cache")
		return next(c)
	}
}

// CORSConfig returns a closed-by-default CORS middleware configuration. An
// empty origin list means CORS is disabled entirely by the caller.
func CORSConfig(origins []string) echo.MiddlewareFunc {
	return echomiddleware.CORSWithConfig(echomiddleware.CORSConfig{
		AllowOrigins:     origins,
		AllowMethods:     []string{echo.GET, echo.POST, echo.PUT, echo.PATCH, echo.DELETE, echo.OPTIONS},
		AllowHeaders:     []string{echo.HeaderOrigin, echo.HeaderAccept, echo.HeaderContentType, echo.HeaderAuthorization, "If-Match", "Idempotency-Key"},
		ExposeHeaders:    []string{"ETag", echo.HeaderRetryAfter, "Idempotency-Replayed"},
		AllowCredentials: false,
		MaxAge:           600,
	})
}
