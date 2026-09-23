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
	e.Use(middleware.RequestID(), middleware.Recover())

	healthHandler := handlers.NewHealthHandler(database)
	authHandler := handlers.NewAuthHandler(database, jwtTokens, cfg.JWTRefreshTokenTTL)
	e.GET("/health", healthHandler.Health)
	e.POST("/auth/login", authHandler.Login)
	e.POST("/auth/refresh", authHandler.Refresh)
	e.POST("/auth/logout", authHandler.Logout, launmiddleware.Authenticate(database, jwtTokens))
	e.GET("/auth/me", authHandler.Me, launmiddleware.Authenticate(database, jwtTokens))

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
