package security

import (
	"testing"
	"time"
)

func TestPlatformTokenAudienceAndKindAreIsolated(t *testing.T) {
	tenant, err := NewTokenManager(testSigningSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	platform, err := NewPlatformTokenManager("different-platform-secret-at-least-32-bytes", time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	now := time.Now()
	platformRaw, _, err := platform.NewAccessToken(1, 10, now)
	if err != nil {
		t.Fatal(err)
	}
	if claims, err := platform.ParseAccessToken(platformRaw); err != nil || claims.Kind != "platform_access" || claims.Role != "PLATFORM_ADMIN" {
		t.Fatalf("platform token invalid: claims=%+v err=%v", claims, err)
	}
	if _, err := tenant.ParseAccessToken(platformRaw); err == nil {
		t.Fatal("tenant parser accepted platform token")
	}
	tenantRaw, _, err := tenant.NewAccessToken(1, 2, 3, "ADMIN", nil, now)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := platform.ParseAccessToken(tenantRaw); err == nil {
		t.Fatal("platform parser accepted tenant token")
	}
	oldRaw, _, err := platform.NewAccessToken(1, 10, now.Add(-2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := platform.ParseAccessToken(oldRaw); err == nil {
		t.Fatal("platform parser accepted expired token")
	}
}
