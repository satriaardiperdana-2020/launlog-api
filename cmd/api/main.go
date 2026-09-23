package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"

	"github.com/satriaardiperdana-2020/launlog-api/internal/config"
	"github.com/satriaardiperdana-2020/launlog-api/internal/handlers"
	launmiddleware "github.com/satriaardiperdana-2020/launlog-api/internal/middleware"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
)

func main() {
	logger := slog.New(slog.NewJSONHandler(os.Stdout, nil))

	if err := run(logger); err != nil {
		logger.Error("application stopped", "error", err)
		os.Exit(1)
	}
}

func run(logger *slog.Logger) error {
	cfg, err := config.Load()
	if err != nil {
		return fmt.Errorf("load configuration: %w", err)
	}

	appContext, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	database, err := repository.NewPostgres(
		appContext,
		cfg.DatabaseURL,
		cfg.Timezone,
		cfg.DatabaseConnectTimeout,
		cfg.DatabaseMaxConnections,
		cfg.DatabaseMinConnections,
	)
	if err != nil {
		return fmt.Errorf("connect to PostgreSQL: %w", err)
	}
	defer database.Close()
	if cfg.Environment == "production" {
		privilegeContext, cancel := context.WithTimeout(appContext, cfg.DatabaseConnectTimeout)
		defer cancel()
		if err := database.VerifyLeastPrivilege(privilegeContext); err != nil {
			return fmt.Errorf("verify PostgreSQL runtime role: %w", err)
		}
	}
	jwtTokens, err := security.NewTokenManager(cfg.JWTSigningSecret, cfg.JWTAccessTokenTTL)
	if err != nil {
		return fmt.Errorf("configure JWT: %w", err)
	}

	e := echo.New()
	e.HideBanner = true
	e.HidePort = true
	// Trust only the direct TCP peer. Reverse proxies must enforce their own
	// limits; untrusted X-Forwarded-For/Real-IP headers never select a key.
	e.IPExtractor = echo.ExtractIPDirect()
	e.Use(middleware.RequestID(), middleware.Recover(), launmiddleware.SecurityHeaders)
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
	outletReports.GET("/:outletId/reports/income", reportsHandler.Get("income"))
	outletReports.GET("/:outletId/reports/expenses", reportsHandler.Get("expenses"))
	outletReports.GET("/:outletId/reports/profit-loss", reportsHandler.Get("profit-loss"))
	outletReports.GET("/:outletId/reports/orders", reportsHandler.Get("orders"))
	outletReports.GET("/:outletId/reports/cancellations", reportsHandler.Get("cancellations"))
	outletReports.GET("/:outletId/reports/customers", reportsHandler.Get("customers"))

	server := &http.Server{
		Addr:              cfg.HTTPAddress(),
		Handler:           e,
		ReadHeaderTimeout: cfg.HTTPReadTimeout,
		ReadTimeout:       cfg.HTTPReadTimeout,
		WriteTimeout:      cfg.HTTPWriteTimeout,
		IdleTimeout:       cfg.HTTPIdleTimeout,
	}

	serverErrors := make(chan error, 1)
	go func() {
		logger.Info("HTTP server starting", "address", server.Addr, "environment", cfg.Environment)
		serverErrors <- server.ListenAndServe()
	}()

	select {
	case <-appContext.Done():
		logger.Info("shutdown signal received")
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
		return nil
	}

	shutdownContext, cancel := context.WithTimeout(context.Background(), cfg.ShutdownTimeout)
	defer cancel()

	if err := server.Shutdown(shutdownContext); err != nil {
		if closeErr := server.Close(); closeErr != nil {
			logger.Error("force close HTTP server", "error", closeErr)
		}
		return fmt.Errorf("gracefully shut down HTTP server: %w", err)
	}

	select {
	case err := <-serverErrors:
		if !errors.Is(err, http.ErrServerClosed) {
			return fmt.Errorf("serve HTTP: %w", err)
		}
	case <-time.After(cfg.ShutdownTimeout):
		return errors.New("HTTP server did not stop before shutdown timeout")
	}

	logger.Info("HTTP server stopped")
	return nil
}
