package server

import (
	"net/http"
	"strings"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
	swaggerFiles "github.com/swaggo/files/v2"

	contract "github.com/satriaardiperdana-2020/launlog-api/api"
	"github.com/satriaardiperdana-2020/launlog-api/internal/config"
	"github.com/satriaardiperdana-2020/launlog-api/internal/handlers"
	launmiddleware "github.com/satriaardiperdana-2020/launlog-api/internal/middleware"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
)

// New builds the same production router used by the executable and API tests.
func New(cfg config.Config, database *repository.Postgres, jwtTokens *security.TokenManager) *echo.Echo {
	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	// Trust only the direct TCP peer. Reverse proxies must enforce their own
	// limits; untrusted X-Forwarded-For/Real-IP headers never select a key.
	e.IPExtractor = echo.ExtractIPDirect()
	e.Use(middleware.RequestID(), middleware.Recover(), launmiddleware.SecurityHeaders)
	if cfg.Environment == "development" {
		registerSwagger(e)
	}
	if len(cfg.CORSAllowedOrigins) > 0 {
		e.Use(launmiddleware.CORSConfig(cfg.CORSAllowedOrigins))
	}

	healthHandler := handlers.NewHealthHandler(database, cfg.ReadinessTimeout)
	authHandler := handlers.NewAuthHandler(database, jwtTokens, cfg.JWTRefreshTokenTTL)
	managementHandler := handlers.NewManagementHandler(database)
	customerHandler := handlers.NewCustomerHandler(database)
	serviceHandler := handlers.NewServiceHandler(database)
	perfumeHandler := handlers.NewPerfumeHandler(database)
	orderHandler := handlers.NewOrderHandler(database)
	paymentHandler := handlers.NewPaymentHandler(database)
	expenseHandler := handlers.NewExpenseHandler(database)
	dashboardHandler := handlers.NewDashboardHandler(database)
	reportsHandler := handlers.NewReportsHandler(database)
	receiptsHandler := handlers.NewReceiptsHandler(database)
	authLimiter := launmiddleware.NewAuthLimiter(4096, 10, time.Minute)
	authBody := launmiddleware.AuthBodyLimit(4096)
	e.GET("/livez", healthHandler.Live)
	e.GET("/readyz", healthHandler.Ready)
	e.GET("/health", healthHandler.Health)
	e.POST("/auth/login", authHandler.Login, authBody, authLimiter.Middleware, launmiddleware.NoStore)
	e.POST("/auth/refresh", authHandler.Refresh, authBody, authLimiter.Middleware, launmiddleware.NoStore)
	e.POST("/auth/logout", authHandler.Logout, authBody, launmiddleware.AuthenticateForLogout(database, jwtTokens), launmiddleware.NoStore)
	e.GET("/auth/me", authHandler.Me, launmiddleware.Authenticate(database, jwtTokens), launmiddleware.NoStore)

	owner := e.Group("", launmiddleware.Authenticate(database, jwtTokens), launmiddleware.RequireAdmin())
	owner.GET("/outlets", managementHandler.ListOutlets)
	owner.POST("/outlets", managementHandler.CreateOutlet)
	owner.GET("/outlets/:outletId", managementHandler.GetOutlet)
	owner.PUT("/outlets/:outletId", managementHandler.UpdateOutlet)
	owner.GET("/staff", managementHandler.ListStaff)
	owner.POST("/staff", managementHandler.CreateStaff)
	owner.GET("/staff/:userId", managementHandler.GetStaff)
	owner.PUT("/staff/:userId", managementHandler.UpdateStaff)
	owner.PUT("/staff/:userId/outlets", managementHandler.ReplaceStaffOutlets)
	owner.GET("/permissions", managementHandler.ListPermissions)
	owner.PUT("/staff/:userId/permissions", managementHandler.ReplaceStaffPermissions)

	customers := e.Group("/customers", launmiddleware.Authenticate(database, jwtTokens))
	customers.GET("", customerHandler.List, launmiddleware.RequirePermission("CUSTOMERS_READ"))
	customers.POST("", customerHandler.Create, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("CUSTOMERS_WRITE"))
	customers.GET("/:customerId", customerHandler.Get, launmiddleware.RequirePermission("CUSTOMERS_READ"))
	customers.PUT("/:customerId", customerHandler.Update, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("CUSTOMERS_WRITE"))
	customers.DELETE("/:customerId", customerHandler.Deactivate, launmiddleware.RequirePermission("CUSTOMERS_WRITE"))
	customers.GET("/:customerId/orders", customerHandler.OrderHistory, launmiddleware.RequirePermission("CUSTOMERS_READ"), launmiddleware.RequirePermission("ORDERS_READ"))

	services := e.Group("/services", launmiddleware.Authenticate(database, jwtTokens))
	services.GET("", serviceHandler.List, launmiddleware.RequirePermission("SERVICES_READ"))
	services.POST("", serviceHandler.Create, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("SERVICES_WRITE"))
	services.GET("/:serviceId", serviceHandler.Get, launmiddleware.RequirePermission("SERVICES_READ"))
	services.PUT("/:serviceId", serviceHandler.Update, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("SERVICES_WRITE"))
	services.DELETE("/:serviceId", serviceHandler.Delete, launmiddleware.RequirePermission("SERVICES_WRITE"))

	perfumes := e.Group("/perfumes", launmiddleware.Authenticate(database, jwtTokens))
	perfumes.GET("", perfumeHandler.List, launmiddleware.RequirePermission("PERFUMES_READ"))
	perfumes.POST("", perfumeHandler.Create, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("PERFUMES_WRITE"))
	perfumes.GET("/:perfumeId", perfumeHandler.Get, launmiddleware.RequirePermission("PERFUMES_READ"))
	perfumes.PUT("/:perfumeId", perfumeHandler.Update, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("PERFUMES_WRITE"))
	perfumes.DELETE("/:perfumeId", perfumeHandler.Delete, launmiddleware.RequirePermission("PERFUMES_WRITE"))

	orders := e.Group("/orders", launmiddleware.Authenticate(database, jwtTokens))
	orders.GET("", orderHandler.List, launmiddleware.RequirePermission("ORDERS_READ"))
	orders.POST("", orderHandler.Create, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("ORDERS_CREATE"))
	orders.GET("/:orderId", orderHandler.Get, launmiddleware.RequirePermission("ORDERS_READ"))
	orders.GET("/:orderId/receipt", receiptsHandler.GetOrderReceipt, launmiddleware.RequirePermission("ORDERS_READ"))
	orders.PATCH("/:orderId/status", orderHandler.TransitionStatus, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("ORDERS_UPDATE"))
	orders.POST("/:orderId/cancel", orderHandler.Cancel, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("ORDERS_UPDATE"))
	orders.GET("/:orderId/status-history", orderHandler.StatusHistory, launmiddleware.RequirePermission("ORDERS_READ"))
	orders.GET("/:orderId/payments", paymentHandler.List, launmiddleware.RequirePermission("PAYMENTS_READ"))
	orders.POST("/:orderId/payments", paymentHandler.Record, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("PAYMENTS_RECORD"))
	orders.POST("/:orderId/payments/:paymentId/void", paymentHandler.Void, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("PAYMENTS_RECORD"))

	expenseCategories := e.Group("/expense-categories", launmiddleware.Authenticate(database, jwtTokens))
	expenseCategories.GET("", expenseHandler.ListCategories, launmiddleware.RequirePermission("EXPENSES_READ"))
	expenseCategories.POST("", expenseHandler.CreateCategory, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("EXPENSES_WRITE"))
	expenseCategories.GET("/:categoryId", expenseHandler.GetCategory, launmiddleware.RequirePermission("EXPENSES_READ"))
	expenseCategories.PUT("/:categoryId", expenseHandler.UpdateCategory, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("EXPENSES_WRITE"))

	expenses := e.Group("/expenses", launmiddleware.Authenticate(database, jwtTokens))
	expenses.GET("", expenseHandler.List, launmiddleware.RequirePermission("EXPENSES_READ"))
	expenses.POST("", expenseHandler.Create, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("EXPENSES_WRITE"))
	expenses.GET("/:expenseId", expenseHandler.Get, launmiddleware.RequirePermission("EXPENSES_READ"))
	expenses.PUT("/:expenseId", expenseHandler.Update, middleware.BodyLimit("64K"), launmiddleware.RequirePermission("EXPENSES_WRITE"))

	outletDashboards := e.Group("/outlets", launmiddleware.Authenticate(database, jwtTokens))
	outletDashboards.GET("/:outletId/receipts/qr/:qrId", receiptsHandler.GetReceiptByQR, launmiddleware.RequirePermission("ORDERS_READ"))
	outletDashboards.GET("/:outletId/receipt-templates", receiptsHandler.ListTemplates, launmiddleware.RequirePermission("RECEIPTS_MANAGE"))
	outletDashboards.POST("/:outletId/receipt-templates", receiptsHandler.CreateTemplate, middleware.BodyLimit("16K"), launmiddleware.RequirePermission("RECEIPTS_MANAGE"))
	outletDashboards.GET("/:outletId/receipt-templates/:templateId", receiptsHandler.GetTemplate, launmiddleware.RequirePermission("RECEIPTS_MANAGE"))
	outletDashboards.PUT("/:outletId/receipt-templates/:templateId", receiptsHandler.UpdateTemplate, middleware.BodyLimit("16K"), launmiddleware.RequirePermission("RECEIPTS_MANAGE"))
	outletDashboards.DELETE("/:outletId/receipt-templates/:templateId", receiptsHandler.DeleteTemplate, launmiddleware.RequirePermission("RECEIPTS_MANAGE"))
	outletDashboards.GET("/:outletId/dashboard", dashboardHandler.GetOutlet, launmiddleware.RequirePermission("REPORTS_READ"))
	outletReports := e.Group("/outlets", launmiddleware.Authenticate(database, jwtTokens), launmiddleware.RequirePermission("REPORTS_READ"))
	for _, reportType := range []string{"income", "expenses", "profit-loss", "orders", "cancellations", "customers"} {
		outletReports.GET("/:outletId/reports/"+reportType, reportsHandler.Get(reportType))
	}

	return e
}

// registerSwagger exposes the checked-in OpenAPI 3 contract and locally
// embedded Swagger UI assets in development only. This keeps production
// responses JSON/API-only and avoids a runtime CDN dependency.
func registerSwagger(e *echo.Echo) {
	e.GET("/openapi.yaml", func(c echo.Context) error {
		specification, err := contract.OpenAPISpec.ReadFile("openapi.yaml")
		if err != nil {
			return echo.NewHTTPError(http.StatusInternalServerError)
		}
		return c.Blob(http.StatusOK, "application/yaml; charset=utf-8", specification)
	})
	e.GET("/swagger", func(c echo.Context) error {
		return c.Redirect(http.StatusTemporaryRedirect, "/swagger/")
	})
	staticFiles := http.FileServer(http.FS(swaggerFiles.FS))
	e.GET("/swagger/swagger-init.js", func(c echo.Context) error {
		c.Response().Header().Set(echo.HeaderContentType, "application/javascript; charset=utf-8")
		c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
		return c.String(http.StatusOK, swaggerInit)
	})
	e.GET("/swagger/*", func(c echo.Context) error {
		assetPath := strings.TrimPrefix(c.Param("*"), "/")
		if assetPath == "" {
			return swaggerPage(c)
		}
		request := c.Request().Clone(c.Request().Context())
		request.URL.Path = "/" + assetPath
		staticFiles.ServeHTTP(c.Response(), request)
		return nil
	})
}

func swaggerPage(c echo.Context) error {
	c.Response().Header().Set(echo.HeaderContentSecurityPolicy, "default-src 'none'; script-src 'self'; style-src 'self' 'unsafe-inline'; img-src 'self' data:; font-src 'self' data:; connect-src 'self'; object-src 'none'; frame-ancestors 'none'; base-uri 'none'")
	c.Response().Header().Set(echo.HeaderCacheControl, "no-cache")
	return c.HTML(http.StatusOK, swaggerIndex)
}

const swaggerIndex = `<!doctype html>
<html lang="en">
<head>
  <meta charset="utf-8">
  <meta name="viewport" content="width=device-width, initial-scale=1">
  <title>Launlog API</title>
  <link rel="stylesheet" href="/swagger/swagger-ui.css">
</head>
<body>
  <div id="swagger-ui"></div>
  <script src="/swagger/swagger-ui-bundle.js"></script>
  <script src="/swagger/swagger-ui-standalone-preset.js"></script>
  <script src="/swagger/swagger-init.js"></script>
</body>
</html>`

const swaggerInit = `window.onload = () => {
  window.ui = SwaggerUIBundle({
    url: "/openapi.yaml",
    dom_id: "#swagger-ui",
    deepLinking: true,
    displayRequestDuration: true,
    persistAuthorization: false,
    validatorUrl: null,
    presets: [SwaggerUIBundle.presets.apis, SwaggerUIStandalonePreset],
    layout: "StandaloneLayout"
  });
};`
