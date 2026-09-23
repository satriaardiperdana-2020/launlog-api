package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadDefaults(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("DATABASE_URL", "postgres://user:password@localhost:5432/launlog")
	t.Setenv("JWT_SIGNING_SECRET", "01234567890123456789012345678901")

	cfg, err := loadFromEnvironment()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Environment != "development" {
		t.Errorf("Environment = %q, want development", cfg.Environment)
	}
	if cfg.HTTPAddress() != "0.0.0.0:8080" {
		t.Errorf("HTTPAddress() = %q, want 0.0.0.0:8080", cfg.HTTPAddress())
	}
	if cfg.HTTPReadTimeout != 10*time.Second {
		t.Errorf("HTTPReadTimeout = %v, want 10s", cfg.HTTPReadTimeout)
	}
	if cfg.DatabaseConnectTimeout != 5*time.Second {
		t.Errorf("DatabaseConnectTimeout = %v, want 5s", cfg.DatabaseConnectTimeout)
	}
	if cfg.ReadinessTimeout != 2*time.Second || cfg.Timezone != "Asia/Jakarta" {
		t.Errorf("readiness configuration = (%v, %q), want (2s, Asia/Jakarta)", cfg.ReadinessTimeout, cfg.Timezone)
	}
	if cfg.DatabaseMaxConnections != 10 || cfg.DatabaseMinConnections != 0 {
		t.Errorf(
			"database connection limits = (%d, %d), want (10, 0)",
			cfg.DatabaseMaxConnections,
			cfg.DatabaseMinConnections,
		)
	}
}

func TestLoadOverrides(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("HTTP_HOST", "127.0.0.1")
	t.Setenv("HTTP_PORT", "9090")
	t.Setenv("HTTP_READ_TIMEOUT", "2s")
	t.Setenv("HTTP_WRITE_TIMEOUT", "3s")
	t.Setenv("HTTP_IDLE_TIMEOUT", "4s")
	t.Setenv("SHUTDOWN_TIMEOUT", "5s")
	t.Setenv("READINESS_TIMEOUT", "1s")
	t.Setenv("APP_TIMEZONE", "Asia/Jakarta")
	t.Setenv("DATABASE_URL", "postgres://user:password@localhost:5432/launlog")
	t.Setenv("JWT_SIGNING_SECRET", "01234567890123456789012345678901")
	t.Setenv("DATABASE_CONNECT_TIMEOUT", "6s")
	t.Setenv("DATABASE_MAX_CONNS", "20")
	t.Setenv("DATABASE_MIN_CONNS", "2")
	t.Setenv("CORS_ALLOWED_ORIGINS", "https://laundry.example, http://localhost:5173")

	cfg, err := loadFromEnvironment()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}

	if cfg.Environment != "test" || cfg.HTTPAddress() != "127.0.0.1:9090" {
		t.Errorf("unexpected application settings: %+v", cfg)
	}
	if cfg.HTTPReadTimeout != 2*time.Second || cfg.HTTPWriteTimeout != 3*time.Second || cfg.HTTPIdleTimeout != 4*time.Second {
		t.Errorf("unexpected HTTP timeouts: %+v", cfg)
	}
	if cfg.ShutdownTimeout != 5*time.Second || cfg.ReadinessTimeout != time.Second || cfg.DatabaseConnectTimeout != 6*time.Second {
		t.Errorf("unexpected lifecycle timeouts: %+v", cfg)
	}
	if cfg.DatabaseMaxConnections != 20 || cfg.DatabaseMinConnections != 2 {
		t.Errorf("unexpected database limits: %+v", cfg)
	}
	if strings.Join(cfg.CORSAllowedOrigins, ",") != "https://laundry.example,http://localhost:5173" {
		t.Fatalf("CORS origins = %#v", cfg.CORSAllowedOrigins)
	}
}

func TestCORSOriginsRejectWildcardAndPaths(t *testing.T) {
	for _, value := range []string{"*", "https://laundry.example/path", "https://user:pass@example.com", "file://local"} {
		if _, err := corsOrigins(value); err == nil {
			t.Errorf("corsOrigins(%q) accepted", value)
		}
	}
	if origins, err := corsOrigins(""); err != nil || len(origins) != 0 {
		t.Fatalf("empty allowlist should keep CORS disabled: %v, %v", origins, err)
	}
}

func TestProductionRequiresVerifiedDatabaseTLS(t *testing.T) {
	clearConfigEnvironment(t)
	t.Setenv("APP_ENV", "production")
	t.Setenv("DATABASE_URL", "postgres://user:password@db.example/launlog?sslmode=require")
	t.Setenv("JWT_SIGNING_SECRET", "01234567890123456789012345678901")
	if _, err := loadFromEnvironment(); err == nil || !strings.Contains(err.Error(), "sslmode=verify-full") {
		t.Fatalf("production accepted unverified TLS: %v", err)
	}
	t.Setenv("DATABASE_URL", "postgres://user:password@db.example/launlog?sslmode=verify-full")
	if _, err := loadFromEnvironment(); err != nil {
		t.Fatalf("production rejected verified TLS: %v", err)
	}
}

func TestLoadValidation(t *testing.T) {
	tests := []struct {
		name        string
		variable    string
		value       string
		errorPart   string
		setDatabase bool
	}{
		{name: "database URL required", errorPart: "DATABASE_URL is required"},
		{name: "invalid port", variable: "HTTP_PORT", value: "70000", errorPart: "HTTP_PORT", setDatabase: true},
		{name: "invalid duration", variable: "HTTP_READ_TIMEOUT", value: "soon", errorPart: "HTTP_READ_TIMEOUT", setDatabase: true},
		{name: "invalid readiness timeout", variable: "READINESS_TIMEOUT", value: "0s", errorPart: "readiness", setDatabase: true},
		{name: "unsupported timezone", variable: "APP_TIMEZONE", value: "UTC", errorPart: "APP_TIMEZONE", setDatabase: true},
		{name: "invalid maximum connections", variable: "DATABASE_MAX_CONNS", value: "0", errorPart: "DATABASE_MAX_CONNS", setDatabase: true},
		{name: "minimum exceeds maximum", variable: "DATABASE_MIN_CONNS", value: "11", errorPart: "DATABASE_MIN_CONNS", setDatabase: true},
		{name: "public example signing secret", variable: "JWT_SIGNING_SECRET", value: "replace-with-a-random-secret-of-at-least-32-bytes", errorPart: "JWT_SIGNING_SECRET", setDatabase: true},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			clearConfigEnvironment(t)
			if test.setDatabase {
				t.Setenv("DATABASE_URL", "postgres://user:password@localhost:5432/launlog")
				t.Setenv("JWT_SIGNING_SECRET", "01234567890123456789012345678901")
			}
			if test.variable != "" {
				t.Setenv(test.variable, test.value)
			}

			_, err := loadFromEnvironment()
			if err == nil || !strings.Contains(err.Error(), test.errorPart) {
				t.Fatalf("Load() error = %v, want error containing %q", err, test.errorPart)
			}
		})
	}
}

func TestLoadReadsEnvironmentSpecificDotenv(t *testing.T) {
	unsetConfigEnvironment(t)
	t.Setenv("APP_ENV", "test")
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	temporaryDirectory := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(temporaryDirectory, ".env.test"),
		[]byte("DATABASE_URL=postgres://dotenv@localhost:5432/launlog\nJWT_SIGNING_SECRET=01234567890123456789012345678901\nHTTP_PORT=9091\n"),
		0600,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.Chdir(temporaryDirectory); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(workingDirectory) })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DatabaseURL != "postgres://dotenv@localhost:5432/launlog" || cfg.HTTPPort != 9091 {
		t.Fatalf("Load() = %+v, want dotenv values", cfg)
	}
}

func TestLoadPreservesExplicitEnvironment(t *testing.T) {
	unsetConfigEnvironment(t)
	t.Setenv("APP_ENV", "test")
	t.Setenv("DATABASE_URL", "postgres://explicit@localhost:5432/launlog")
	t.Setenv("JWT_SIGNING_SECRET", "01234567890123456789012345678901")
	workingDirectory, err := os.Getwd()
	if err != nil {
		t.Fatalf("os.Getwd() error = %v", err)
	}
	temporaryDirectory := t.TempDir()
	if err := os.WriteFile(
		filepath.Join(temporaryDirectory, ".env.test"),
		[]byte("DATABASE_URL=postgres://dotenv@localhost:5432/launlog\nJWT_SIGNING_SECRET=dotenv-secret-that-must-not-override\n"),
		0600,
	); err != nil {
		t.Fatalf("os.WriteFile() error = %v", err)
	}
	if err := os.Chdir(temporaryDirectory); err != nil {
		t.Fatalf("os.Chdir() error = %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(workingDirectory) })

	cfg, err := Load()
	if err != nil {
		t.Fatalf("Load() error = %v", err)
	}
	if cfg.DatabaseURL != "postgres://explicit@localhost:5432/launlog" {
		t.Fatalf("DatabaseURL = %q, want explicit environment value", cfg.DatabaseURL)
	}
}

func clearConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, name := range []string{
		"APP_ENV",
		"HTTP_HOST",
		"HTTP_PORT",
		"HTTP_READ_TIMEOUT",
		"HTTP_WRITE_TIMEOUT",
		"HTTP_IDLE_TIMEOUT",
		"SHUTDOWN_TIMEOUT",
		"READINESS_TIMEOUT",
		"APP_TIMEZONE",
		"DATABASE_URL",
		"DATABASE_CONNECT_TIMEOUT",
		"DATABASE_MAX_CONNS",
		"DATABASE_MIN_CONNS",
		"JWT_SIGNING_SECRET",
		"JWT_ACCESS_TOKEN_TTL",
		"JWT_REFRESH_TOKEN_TTL",
		"CORS_ALLOWED_ORIGINS",
	} {
		t.Setenv(name, "")
	}
}

func unsetConfigEnvironment(t *testing.T) {
	t.Helper()
	for _, environmentName := range configEnvironmentNames() {
		name := environmentName
		previous, existed := os.LookupEnv(name)
		if err := os.Unsetenv(name); err != nil {
			t.Fatalf("os.Unsetenv(%q) error = %v", name, err)
		}
		t.Cleanup(func() {
			if existed {
				_ = os.Setenv(name, previous)
			} else {
				_ = os.Unsetenv(name)
			}
		})
	}
}

func configEnvironmentNames() []string {
	return []string{
		"APP_ENV",
		"HTTP_HOST",
		"HTTP_PORT",
		"HTTP_READ_TIMEOUT",
		"HTTP_WRITE_TIMEOUT",
		"HTTP_IDLE_TIMEOUT",
		"SHUTDOWN_TIMEOUT",
		"READINESS_TIMEOUT",
		"APP_TIMEZONE",
		"DATABASE_URL",
		"DATABASE_CONNECT_TIMEOUT",
		"DATABASE_MAX_CONNS",
		"DATABASE_MIN_CONNS",
		"JWT_SIGNING_SECRET",
		"JWT_ACCESS_TOKEN_TTL",
		"JWT_REFRESH_TOKEN_TTL",
		"CORS_ALLOWED_ORIGINS",
	}
}
