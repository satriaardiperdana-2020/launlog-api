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
	e.Use(middleware.RequestID(), middleware.Recover())

	healthHandler := handlers.NewHealthHandler(database, cfg.ReadinessTimeout)
	authHandler := handlers.NewAuthHandler(database, jwtTokens, cfg.JWTRefreshTokenTTL)
	managementHandler := handlers.NewManagementHandler(database)
	customerHandler := handlers.NewCustomerHandler(database)
	serviceHandler := handlers.NewServiceHandler(database)
	perfumeHandler := handlers.NewPerfumeHandler(database)
	orderHandler := handlers.NewOrderHandler(database)
	authLimiter := launmiddleware.NewAuthLimiter(4096, 10, time.Minute)
	authBody := launmiddleware.AuthBodyLimit(4096)
	e.GET("/livez", healthHandler.Live)
	e.GET("/readyz", healthHandler.Ready)
	e.GET("/health", healthHandler.Health)
	e.POST("/auth/login", authHandler.Login, authBody, authLimiter.Middleware)
	e.POST("/auth/refresh", authHandler.Refresh, authBody, authLimiter.Middleware)
	e.POST("/auth/logout", authHandler.Logout, authBody, launmiddleware.AuthenticateForLogout(database, jwtTokens))
	e.GET("/auth/me", authHandler.Me, launmiddleware.Authenticate(database, jwtTokens))

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
