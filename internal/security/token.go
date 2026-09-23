package security

import (
	"crypto/rand"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"fmt"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const issuer = "launlog-api"

type AccessClaims struct {
	BusinessID int64   `json:"business_id"`
	Role       string  `json:"role"`
	OutletIDs  []int64 `json:"outlet_ids"`
	SessionID  int64   `json:"sid"`
	jwt.RegisteredClaims
}

type TokenManager struct {
	key       []byte
	accessTTL time.Duration
}

func NewTokenManager(key string, accessTTL time.Duration) (*TokenManager, error) {
	if len(key) < 32 || key == "replace-with-a-random-secret-of-at-least-32-bytes" {
		return nil, errors.New("JWT_SIGNING_SECRET must be a non-placeholder secret of at least 32 bytes")
	}
	return &TokenManager{key: []byte(key), accessTTL: accessTTL}, nil
}

func (m *TokenManager) NewAccessToken(userID, businessID, sessionID int64, role string, outletIDs []int64, now time.Time) (string, time.Time, error) {
	expiresAt := jwt.NewNumericDate(now.Add(m.accessTTL)).Time
	claims := AccessClaims{
		BusinessID: businessID, Role: role, OutletIDs: outletIDs, SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: issuer, Subject: strconv.FormatInt(userID, 10), Audience: []string{issuer},
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	token := jwt.NewWithClaims(jwt.SigningMethodHS256, claims)
	signed, err := token.SignedString(m.key)
	return signed, expiresAt, err
}

func (m *TokenManager) ParseAccessToken(raw string) (*AccessClaims, error) {
	claims := new(AccessClaims)
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected JWT signing algorithm")
		}
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected JWT signing method")
		}
		return m.key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(issuer), jwt.WithAudience(issuer), jwt.WithExpirationRequired())
	if err != nil || !token.Valid {
		return nil, errors.New("invalid access token")
	}
	if claims.BusinessID < 1 || claims.SessionID < 1 || claims.Subject == "" {
		return nil, errors.New("invalid access token claims")
	}
	return claims, nil
}

func NewRefreshToken() (string, string, error) {
	bytes := make([]byte, 32)
	if _, err := rand.Read(bytes); err != nil {
		return "", "", fmt.Errorf("generate refresh token: %w", err)
	}
	raw := base64.RawURLEncoding.EncodeToString(bytes)
	return raw, HashRefreshToken(raw), nil
}

func HashRefreshToken(raw string) string {
	digest := sha256.Sum256([]byte(raw))
	return base64.RawURLEncoding.EncodeToString(digest[:])
}

func SubjectID(claims *AccessClaims) (int64, error) {
	id, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || id < 1 {
		return 0, errors.New("invalid access token subject")
	}
	return id, nil
}
