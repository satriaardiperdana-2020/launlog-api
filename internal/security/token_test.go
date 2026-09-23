package security

import (
	"testing"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const testSigningSecret = "01234567890123456789012345678901"

func TestAccessTokenExpires(t *testing.T) {
	manager, err := NewTokenManager(testSigningSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	token, _, err := manager.NewAccessToken(1, 2, 3, "ADMIN", []int64{4}, time.Now().Add(-2*time.Minute))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ParseAccessToken(token); err == nil {
		t.Fatal("expired token was accepted")
	}
}

func TestAccessTokenRejectsOtherSigningAlgorithms(t *testing.T) {
	manager, err := NewTokenManager(testSigningSecret, time.Minute)
	if err != nil {
		t.Fatal(err)
	}
	claims := AccessClaims{BusinessID: 2, SessionID: 3, RegisteredClaims: jwt.RegisteredClaims{Issuer: issuer, Subject: "1", Audience: []string{issuer}, ExpiresAt: jwt.NewNumericDate(time.Now().Add(time.Minute))}}
	token, err := jwt.NewWithClaims(jwt.SigningMethodHS384, claims).SignedString([]byte(testSigningSecret))
	if err != nil {
		t.Fatal(err)
	}
	if _, err := manager.ParseAccessToken(token); err == nil {
		t.Fatal("HS384 token was accepted")
	}
}

func TestRefreshTokenIsRandomAndOnlyHashIsStored(t *testing.T) {
	raw, hash, err := NewRefreshToken()
	if err != nil {
		t.Fatal(err)
	}
	if raw == hash || HashRefreshToken(raw) != hash {
		t.Fatal("refresh token hash is invalid")
	}
}

func TestTokenManagerRejectsShortSigningSecret(t *testing.T) {
	if _, err := NewTokenManager("short", time.Minute); err == nil {
		t.Fatal("short signing secret was accepted")
	}
}
