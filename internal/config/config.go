package config

import (
	"errors"
	"fmt"
	"net"
	"os"
	"strconv"
	"strings"
	"time"
)

const (
	defaultEnvironment            = "development"
	defaultHTTPHost               = "0.0.0.0"
	defaultHTTPPort               = 8080
	defaultHTTPReadTimeout        = 10 * time.Second
	defaultHTTPWriteTimeout       = 15 * time.Second
	defaultHTTPIdleTimeout        = 60 * time.Second
	defaultShutdownTimeout        = 10 * time.Second
	defaultDatabaseConnectTimeout = 5 * time.Second
	defaultDatabaseMaxConnections = 10
	defaultDatabaseMinConnections = 0
)

// Config contains all process-level settings. It is loaded once at startup.
type Config struct {
	Environment            string
	HTTPHost               string
	HTTPPort               int
	HTTPReadTimeout        time.Duration
	HTTPWriteTimeout       time.Duration
	HTTPIdleTimeout        time.Duration
	ShutdownTimeout        time.Duration
	DatabaseURL            string
	DatabaseConnectTimeout time.Duration
	DatabaseMaxConnections int32
	DatabaseMinConnections int32
}

// Load reads optional dotenv files and validates environment configuration.
func Load() (Config, error) {
	if err := loadDotEnv(); err != nil {
		return Config{}, err
	}
	return loadFromEnvironment()
}

func loadFromEnvironment() (Config, error) {
	cfg := Config{
		Environment: valueOrDefault("APP_ENV", defaultEnvironment),
		HTTPHost:    valueOrDefault("HTTP_HOST", defaultHTTPHost),
		DatabaseURL: os.Getenv("DATABASE_URL"),
	}

	var err error
	if cfg.HTTPPort, err = intValue("HTTP_PORT", defaultHTTPPort); err != nil {
		return Config{}, err
	}
	if cfg.HTTPReadTimeout, err = durationValue("HTTP_READ_TIMEOUT", defaultHTTPReadTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTPWriteTimeout, err = durationValue("HTTP_WRITE_TIMEOUT", defaultHTTPWriteTimeout); err != nil {
		return Config{}, err
	}
	if cfg.HTTPIdleTimeout, err = durationValue("HTTP_IDLE_TIMEOUT", defaultHTTPIdleTimeout); err != nil {
		return Config{}, err
	}
	if cfg.ShutdownTimeout, err = durationValue("SHUTDOWN_TIMEOUT", defaultShutdownTimeout); err != nil {
		return Config{}, err
	}
	if cfg.DatabaseConnectTimeout, err = durationValue("DATABASE_CONNECT_TIMEOUT", defaultDatabaseConnectTimeout); err != nil {
		return Config{}, err
	}

	maxConnections, err := intValue("DATABASE_MAX_CONNS", defaultDatabaseMaxConnections)
	if err != nil {
		return Config{}, err
	}
	minConnections, err := intValue("DATABASE_MIN_CONNS", defaultDatabaseMinConnections)
	if err != nil {
		return Config{}, err
	}
	if maxConnections > int(^uint32(0)>>1) || minConnections > int(^uint32(0)>>1) {
		return Config{}, errors.New("database connection limits exceed int32 range")
	}
	cfg.DatabaseMaxConnections = int32(maxConnections)
	cfg.DatabaseMinConnections = int32(minConnections)

	if err := cfg.validate(); err != nil {
		return Config{}, err
	}

	return cfg, nil
}

// loadDotEnv loads .env.<APP_ENV> first, then .env as a fallback. Existing
// process environment variables always take precedence over dotenv values.
func loadDotEnv() error {
	environment := valueOrDefault("APP_ENV", defaultEnvironment)
	files := []string{".env." + environment, ".env"}

	loaded := make(map[string]struct{})
	for _, filename := range files {
		contents, err := os.ReadFile(filename)
		if errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err != nil {
			return fmt.Errorf("read dotenv file %s: %w", filename, err)
		}

		for lineNumber, line := range strings.Split(string(contents), "\n") {
			name, value, ok := parseDotEnvLine(line)
			if !ok {
				continue
			}
			if _, alreadyLoaded := loaded[name]; alreadyLoaded {
				continue
			}
			if _, explicitlySet := os.LookupEnv(name); explicitlySet {
				loaded[name] = struct{}{}
				continue
			}
			if err := os.Setenv(name, value); err != nil {
				return fmt.Errorf("load dotenv variable %s from %s line %d: %w", name, filename, lineNumber+1, err)
			}
			loaded[name] = struct{}{}
		}
	}

	return nil
}

func parseDotEnvLine(line string) (string, string, bool) {
	line = strings.TrimSpace(line)
	if line == "" || strings.HasPrefix(line, "#") {
		return "", "", false
	}
	line = strings.TrimPrefix(line, "export ")

	name, value, ok := strings.Cut(line, "=")
	if !ok {
		return "", "", false
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return "", "", false
	}

	value = strings.TrimSpace(value)
	if len(value) >= 2 {
		first, last := value[0], value[len(value)-1]
		if (first == '\'' && last == '\'') || (first == '"' && last == '"') {
			value = value[1 : len(value)-1]
		}
	}
	return name, value, true
}

// HTTPAddress returns the host and port in net/http listen format.
func (c Config) HTTPAddress() string {
	return net.JoinHostPort(c.HTTPHost, strconv.Itoa(c.HTTPPort))
}

func (c Config) validate() error {
	if c.DatabaseURL == "" {
		return errors.New("DATABASE_URL is required")
	}
	if c.HTTPPort < 1 || c.HTTPPort > 65535 {
		return errors.New("HTTP_PORT must be between 1 and 65535")
	}
	if c.HTTPReadTimeout <= 0 || c.HTTPWriteTimeout <= 0 || c.HTTPIdleTimeout <= 0 {
		return errors.New("HTTP timeouts must be positive")
	}
	if c.ShutdownTimeout <= 0 || c.DatabaseConnectTimeout <= 0 {
		return errors.New("shutdown and database connection timeouts must be positive")
	}
	if c.DatabaseMaxConnections <= 0 {
		return errors.New("DATABASE_MAX_CONNS must be positive")
	}
	if c.DatabaseMinConnections < 0 {
		return errors.New("DATABASE_MIN_CONNS cannot be negative")
	}
	if c.DatabaseMinConnections > c.DatabaseMaxConnections {
		return errors.New("DATABASE_MIN_CONNS cannot exceed DATABASE_MAX_CONNS")
	}

	return nil
}

func valueOrDefault(name, fallback string) string {
	if value, ok := os.LookupEnv(name); ok && value != "" {
		return value
	}
	return fallback
}

func intValue(name string, fallback int) (int, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}

	parsed, err := strconv.Atoi(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be an integer: %w", name, err)
	}
	return parsed, nil
}

func durationValue(name string, fallback time.Duration) (time.Duration, error) {
	value := os.Getenv(name)
	if value == "" {
		return fallback, nil
	}

	parsed, err := time.ParseDuration(value)
	if err != nil {
		return 0, fmt.Errorf("%s must be a duration: %w", name, err)
	}
	return parsed, nil
}
