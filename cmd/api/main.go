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

	"github.com/satriaardiperdana-2020/launlog-api/internal/config"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
	"github.com/satriaardiperdana-2020/launlog-api/internal/server"
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

	e := server.New(cfg, database, jwtTokens)
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
