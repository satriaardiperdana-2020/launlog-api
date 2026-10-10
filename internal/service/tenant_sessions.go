package service

import (
	"context"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
)

type TenantIdentity struct {
	ID, BusinessID        int64
	Email, FullName, Role string
}

// TenantSessions builds credentials inside the caller's transaction. It neither
// commits nor writes HTTP responses, so login and registration remain atomic.
type TenantSessions struct {
	refreshTTL time.Duration
	newRefresh func() (string, string, error)
	newAccess  func(int64, int64, int64, string, []int64, time.Time) (string, time.Time, error)
}

func NewTenantSessions(tokens *security.TokenManager, refreshTTL time.Duration) *TenantSessions {
	sessions := &TenantSessions{refreshTTL: refreshTTL, newRefresh: security.NewRefreshToken}
	if tokens != nil {
		sessions.newAccess = tokens.NewAccessToken
	} else {
		sessions.newAccess = func(int64, int64, int64, string, []int64, time.Time) (string, time.Time, error) {
			return "", time.Time{}, errors.New("tenant token manager is unavailable")
		}
	}
	return sessions
}

func (s *TenantSessions) Create(ctx context.Context, q *db.Queries, u TenantIdentity, outs []int64, perms []string) (map[string]any, int64, error) {
	familyID, err := q.CreateSessionFamily(ctx, db.CreateSessionFamilyParams{BusinessID: u.BusinessID, UserID: u.ID})
	if err != nil {
		return nil, 0, err
	}
	raw, hash, err := s.newRefresh()
	if err != nil {
		return nil, 0, err
	}
	session, err := q.CreateRefreshToken(ctx, db.CreateRefreshTokenParams{BusinessID: u.BusinessID, UserID: u.ID, FamilyID: familyID, TokenHash: hash, ExpiresAt: pgtype.Timestamptz{Time: time.Now().Add(s.refreshTTL), Valid: true}})
	if err != nil {
		return nil, 0, err
	}
	response, err := s.Response(ctx, q, u, session.ID, raw, session.ExpiresAt.Time, outs, perms)
	if err != nil {
		return nil, 0, err
	}
	return response, familyID, nil
}

func (s *TenantSessions) Response(ctx context.Context, q *db.Queries, u TenantIdentity, sid int64, refresh string, refreshExpiry time.Time, outs []int64, perms []string) (map[string]any, error) {
	outlets, err := q.ListUserOutlets(ctx, db.ListUserOutletsParams{BusinessID: u.BusinessID, UserID: u.ID})
	if err != nil {
		return nil, err
	}
	if outlets == nil {
		outlets = []db.ListUserOutletsRow{}
	}
	if outs == nil {
		outs = []int64{}
	}
	if perms == nil {
		perms = []string{}
	}
	access, accessExp, err := s.newAccess(u.ID, u.BusinessID, sid, u.Role, outs, time.Now())
	if err != nil {
		return nil, err
	}
	return map[string]any{"tokens": map[string]any{"accessToken": access, "refreshToken": refresh, "tokenType": "Bearer", "accessTokenExpiresAt": accessExp, "refreshTokenExpiresAt": refreshExpiry}, "user": map[string]any{"outlets": outlets, "id": u.ID, "businessId": u.BusinessID, "email": u.Email, "fullName": u.FullName, "role": u.Role, "outletIds": outs, "permissions": perms}}, nil
}
