package repository

import "testing"

func TestNewPoolConfigAppliesFoundationSettings(t *testing.T) {
	poolConfig, err := newPoolConfig(
		"postgres://user:password@localhost:5432/launlog?sslmode=disable",
		"Asia/Jakarta",
		12,
		2,
	)
	if err != nil {
		t.Fatalf("newPoolConfig() error = %v", err)
	}
	if poolConfig.MaxConns != 12 || poolConfig.MinConns != 2 {
		t.Fatalf("pool limits = (%d, %d), want (12, 2)", poolConfig.MaxConns, poolConfig.MinConns)
	}
	if poolConfig.ConnConfig.RuntimeParams["application_name"] != "launlog-api" {
		t.Fatalf("application_name = %q, want launlog-api", poolConfig.ConnConfig.RuntimeParams["application_name"])
	}
	if poolConfig.ConnConfig.RuntimeParams["timezone"] != "Asia/Jakarta" {
		t.Fatalf("timezone = %q, want Asia/Jakarta", poolConfig.ConnConfig.RuntimeParams["timezone"])
	}
}

func TestNewPoolConfigSanitizesInvalidURL(t *testing.T) {
	if _, err := newPoolConfig("postgres://%", "Asia/Jakarta", 10, 0); err == nil || err.Error() != "invalid PostgreSQL configuration" {
		t.Fatalf("newPoolConfig() error = %v, want sanitized configuration error", err)
	}
}
