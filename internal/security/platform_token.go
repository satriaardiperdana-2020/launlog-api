package security

import (
	"errors"
	"strconv"
	"time"

	"github.com/golang-jwt/jwt/v5"
)

const platformAudience = "launlog-platform"

type PlatformClaims struct {
	Kind      string `json:"kind"`
	Role      string `json:"role"`
	SessionID int64  `json:"sid"`
	jwt.RegisteredClaims
}

type PlatformTokenManager struct {
	key       []byte
	accessTTL time.Duration
}

func NewPlatformTokenManager(key string, accessTTL time.Duration) (*PlatformTokenManager, error) {
	if len(key) < 32 || key == "replace-with-a-random-secret-of-at-least-32-bytes" {
		return nil, errors.New("PLATFORM_JWT_SIGNING_SECRET must be a non-placeholder secret of at least 32 bytes")
	}
	return &PlatformTokenManager{key: []byte(key), accessTTL: accessTTL}, nil
}

func (m *PlatformTokenManager) NewAccessToken(adminID, sessionID int64, now time.Time) (string, time.Time, error) {
	expiresAt := jwt.NewNumericDate(now.Add(m.accessTTL)).Time
	claims := PlatformClaims{
		Kind: "platform_access", Role: "PLATFORM_ADMIN", SessionID: sessionID,
		RegisteredClaims: jwt.RegisteredClaims{
			Issuer: issuer, Subject: strconv.FormatInt(adminID, 10), Audience: []string{platformAudience},
			IssuedAt: jwt.NewNumericDate(now), ExpiresAt: jwt.NewNumericDate(expiresAt),
		},
	}
	signed, err := jwt.NewWithClaims(jwt.SigningMethodHS256, claims).SignedString(m.key)
	return signed, expiresAt, err
}

func (m *PlatformTokenManager) ParseAccessToken(raw string) (*PlatformClaims, error) {
	claims := new(PlatformClaims)
	token, err := jwt.ParseWithClaims(raw, claims, func(token *jwt.Token) (interface{}, error) {
		if token.Method.Alg() != jwt.SigningMethodHS256.Alg() {
			return nil, errors.New("unexpected JWT signing algorithm")
		}
		if _, ok := token.Method.(*jwt.SigningMethodHMAC); !ok {
			return nil, errors.New("unexpected JWT signing method")
		}
		return m.key, nil
	}, jwt.WithValidMethods([]string{jwt.SigningMethodHS256.Alg()}), jwt.WithIssuer(issuer), jwt.WithAudience(platformAudience), jwt.WithExpirationRequired())
	if err != nil || !token.Valid || claims.Kind != "platform_access" || claims.Role != "PLATFORM_ADMIN" || claims.SessionID < 1 {
		return nil, errors.New("invalid platform access token")
	}
	adminID, err := strconv.ParseInt(claims.Subject, 10, 64)
	if err != nil || adminID != 1 {
		return nil, errors.New("invalid platform access token subject")
	}
	return claims, nil
}
