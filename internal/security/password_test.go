package security

import (
	"strings"
	"testing"

	"golang.org/x/crypto/bcrypt"
)

func TestPasswordCreationPolicyAndBcryptByteLimit(t *testing.T) {
	for _, password := range []string{"short", strings.Repeat("é", 37), strings.Repeat("a", 129), ""} {
		if _, err := HashPassword(password); err == nil {
			t.Fatalf("accepted invalid new password of %d bytes", len(password))
		}
	}
	password := strings.Repeat("é", 36)
	hash, err := HashPassword(password)
	if err != nil || VerifyPassword(hash, password) != nil {
		t.Fatalf("72-byte UTF-8 password failed: %v", err)
	}
	VerifyDummyPassword("wrong-password")
	if cost, err := bcrypt.Cost([]byte(dummyPasswordHash)); err != nil || cost != bcryptCost {
		t.Fatalf("dummy bcrypt hash must be valid at cost %d: cost=%d err=%v", bcryptCost, cost, err)
	}
	if err := bcrypt.CompareHashAndPassword([]byte(dummyPasswordHash), []byte("dummy-password-constant")); err != nil {
		t.Fatalf("dummy bcrypt hash does not verify: %v", err)
	}
}
