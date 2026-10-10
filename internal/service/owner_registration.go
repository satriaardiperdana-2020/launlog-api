package service

import (
	"context"
	"encoding/json"
	"errors"
	"time"

	"github.com/jackc/pgx/v5/pgconn"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	db "github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
)

type OwnerRegistrationInput struct {
	Email       string           `json:"email"`
	Password    string           `json:"password"`
	FullName    string           `json:"fullName"`
	Business    BusinessInput    `json:"business"`
	FirstOutlet FirstOutletInput `json:"firstOutlet"`
}

type OwnerRegistration struct {
	database *repository.Postgres
	sessions *TenantSessions
}

func NewOwnerRegistration(database *repository.Postgres, tokens *security.TokenManager, refreshTTL time.Duration) *OwnerRegistration {
	return &OwnerRegistration{database: database, sessions: NewTenantSessions(tokens, refreshTTL)}
}

func (s *OwnerRegistration) Register(ctx context.Context, req OwnerRegistrationInput) (map[string]any, error) {
	in, err := normalizeProvisionInput(ProvisionBusinessInput{Business: req.Business, FirstOutlet: req.FirstOutlet, FirstAdmin: FirstAdminInput{Email: req.Email, FullName: req.FullName, Password: req.Password}})
	if err != nil {
		return nil, err
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	hash, err := security.HashPassword(in.FirstAdmin.Password)
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer func() {
		rollbackCtx, rollbackCancel := context.WithTimeout(context.Background(), 3*time.Second)
		defer rollbackCancel()
		_ = tx.Rollback(rollbackCtx)
	}()
	q := s.database.Queries().WithTx(tx)
	tenant, err := createTenant(ctx, q, in, hash)
	if err != nil {
		var pgErr *pgconn.PgError
		if errors.As(err, &pgErr) && pgErr.Code == "23505" && pgErr.ConstraintName == "users_email_uq" {
			return nil, ErrOwnerEmailUnavailable
		}
		return nil, err
	}
	u := tenant.FirstAdmin
	response, familyID, err := s.sessions.Create(ctx, q, TenantIdentity{ID: u.ID, BusinessID: u.BusinessID, Email: u.Email, FullName: u.FullName, Role: u.Role}, []int64{tenant.FirstOutlet.ID}, []string{})
	if err != nil {
		return nil, err
	}
	metadata, err := json.Marshal(map[string]any{"ownerId": u.ID, "outletId": tenant.FirstOutlet.ID, "outletTimezone": tenant.FirstOutlet.Timezone, "role": u.Role})
	if err != nil {
		return nil, err
	}
	if err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{BusinessID: u.BusinessID, ActorUserID: nullableID(u.ID), Action: "OWNER_REGISTERED", EntityType: "business", EntityID: nullableID(u.BusinessID), NewValues: metadata}); err != nil {
		return nil, err
	}
	if err := q.InsertAuditLog(ctx, db.InsertAuditLogParams{BusinessID: u.BusinessID, ActorUserID: nullableID(u.ID), Action: "AUTH_SESSION_CREATED", EntityType: "session_family", EntityID: nullableID(familyID), NewValues: []byte(`{"event":"AUTH_SESSION_CREATED"}`)}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return response, nil
}
