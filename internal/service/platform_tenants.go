package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/mail"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	db "github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
)

var (
	ErrPlatformInvalid       = errors.New("invalid platform request")
	ErrPlatformDenied        = errors.New("platform operation denied")
	ErrPlatformNotFound      = errors.New("platform target not found")
	ErrPlatformConflict      = errors.New("platform operation conflicts")
	ErrAdminEmailUnavailable = errors.New("administrator email unavailable")
)

type PlatformActor struct {
	ID, SessionID int64
	RequestID     string
}
type PlatformTenants struct{ database *repository.Postgres }

func NewPlatformTenants(d *repository.Postgres) *PlatformTenants {
	return &PlatformTenants{database: d}
}
func nullableID(id int64) pgtype.Int8   { return pgtype.Int8{Int64: id, Valid: id > 0} }
func nullableText(s string) pgtype.Text { return pgtype.Text{String: s, Valid: s != ""} }
func optionalPlatformText(s *string) pgtype.Text {
	if s == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: strings.TrimSpace(*s), Valid: true}
}
func platformDBError(err error) error {
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPlatformNotFound
	}
	var e *pgconn.PgError
	if errors.As(err, &e) && e.Code == "23505" {
		if e.ConstraintName == "users_email_uq" {
			return ErrAdminEmailUnavailable
		}
		return ErrPlatformConflict
	}
	return err
}

// Both audit streams share the caller's transaction. Never pass request bodies.
func platformBusinessAudit(ctx context.Context, q *db.Queries, a PlatformActor, owner, business, session int64, reason, action, target string, id int64, changes map[string]any) error {
	if changes == nil {
		changes = map[string]any{}
	}
	if owner > 0 {
		changes["ownerId"] = owner
	}
	payload, err := json.Marshal(changes)
	if err != nil {
		return err
	}
	auditID, err := q.PlatformWriteTenantAudit(ctx, db.PlatformWriteTenantAuditParams{ActorPlatformAdminID: nullableID(a.ID), BusinessID: nullableID(business), SupportSessionID: nullableID(session), Reason: nullableText(reason), Action: action, TargetType: target, TargetID: nullableID(id), RequestID: nullableText(a.RequestID), Metadata: payload})
	if err != nil {
		return err
	}
	if business == 0 {
		return nil
	}
	return q.BusinessWritePlatformAudit(ctx, db.BusinessWritePlatformAuditParams{BusinessID: business, ActorUserID: nullableID(owner), ActorPlatformAdminID: nullableID(a.ID), SupportSessionID: nullableID(session), PlatformAuditID: nullableID(auditID), Action: action, EntityType: target, EntityID: nullableID(id), NewValues: payload})
}

// The session-family lock also serializes privileged operations with logout.
func (s *PlatformTenants) platformTx(ctx context.Context, a PlatformActor, run func(context.Context, *db.Queries, int64) (any, error)) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	family, err := q.SupportGetPlatformFamily(ctx, db.SupportGetPlatformFamilyParams{ID: a.SessionID, PlatformAdminID: a.ID})
	if err != nil {
		return nil, ErrPlatformDenied
	}
	result, err := run(ctx, q, family)
	if err != nil {
		return nil, platformDBError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

type ProvisionBusinessInput struct {
	Business struct {
		Name    string  `json:"name"`
		Phone   *string `json:"phone"`
		Address *string `json:"address"`
	} `json:"business"`
	FirstOutlet struct {
		Code    string  `json:"code"`
		Name    string  `json:"name"`
		Phone   *string `json:"phone"`
		Address *string `json:"address"`
	} `json:"firstOutlet"`
	FirstAdmin struct {
		Email    string `json:"email"`
		FullName string `json:"fullName"`
		Password string `json:"password"`
	} `json:"firstAdmin"`
}

func validOptional(s *string, max int) bool { return s == nil || len([]rune(*s)) <= max }
func (s *PlatformTenants) Provision(ctx context.Context, a PlatformActor, in ProvisionBusinessInput) (any, error) {
	in.Business.Name = strings.TrimSpace(in.Business.Name)
	in.FirstOutlet.Code = strings.TrimSpace(in.FirstOutlet.Code)
	in.FirstOutlet.Name = strings.TrimSpace(in.FirstOutlet.Name)
	in.FirstAdmin.Email = strings.ToLower(strings.TrimSpace(in.FirstAdmin.Email))
	in.FirstAdmin.FullName = strings.TrimSpace(in.FirstAdmin.FullName)
	email, err := mail.ParseAddress(in.FirstAdmin.Email)
	if err != nil || email.Address != in.FirstAdmin.Email || len(in.FirstAdmin.Email) > 254 || in.Business.Name == "" || len([]rune(in.Business.Name)) > 200 || in.FirstOutlet.Code == "" || len([]rune(in.FirstOutlet.Code)) > 64 || in.FirstOutlet.Name == "" || len([]rune(in.FirstOutlet.Name)) > 200 || in.FirstAdmin.FullName == "" || len([]rune(in.FirstAdmin.FullName)) > 200 || !validOptional(in.Business.Phone, 32) || !validOptional(in.Business.Address, 500) || !validOptional(in.FirstOutlet.Phone, 32) || !validOptional(in.FirstOutlet.Address, 500) {
		return nil, ErrPlatformInvalid
	}
	hash, err := security.HashPassword(in.FirstAdmin.Password)
	if err != nil {
		return nil, ErrPlatformInvalid
	}
	return s.platformTx(ctx, a, func(ctx context.Context, q *db.Queries, _ int64) (any, error) {
		b, err := q.PlatformCreateBusiness(ctx, db.PlatformCreateBusinessParams{Name: in.Business.Name, Phone: optionalPlatformText(in.Business.Phone), Address: optionalPlatformText(in.Business.Address)})
		if err != nil {
			return nil, err
		}
		o, err := q.CreateOutlet(ctx, db.CreateOutletParams{BusinessID: b.ID, Code: in.FirstOutlet.Code, Name: in.FirstOutlet.Name, Phone: optionalPlatformText(in.FirstOutlet.Phone), Address: optionalPlatformText(in.FirstOutlet.Address)})
		if err != nil {
			return nil, err
		}
		u, err := q.PlatformCreateFirstAdmin(ctx, db.PlatformCreateFirstAdminParams{BusinessID: b.ID, Email: in.FirstAdmin.Email, FullName: in.FirstAdmin.FullName, PasswordHash: hash})
		if err != nil {
			return nil, err
		}
		if err := q.AddStaffOutlet(ctx, db.AddStaffOutletParams{BusinessID: b.ID, UserID: u.ID, OutletID: o.ID}); err != nil {
			return nil, err
		}
		if err := platformBusinessAudit(ctx, q, a, 0, b.ID, 0, "Tenant onboarding", "BUSINESS_PROVISIONED", "business", b.ID, map[string]any{"outletId": o.ID, "adminId": u.ID, "role": "ADMIN"}); err != nil {
			return nil, err
		}
		return map[string]any{"business": b, "firstOutlet": o, "firstAdmin": u}, nil
	})
}
func (s *PlatformTenants) Businesses(ctx context.Context, a PlatformActor, id int64, search string, active pgtype.Bool, limit, offset int32) (any, error) {
	return s.platformTx(ctx, a, func(ctx context.Context, q *db.Queries, _ int64) (any, error) {
		var result any
		if id > 0 {
			b, err := q.PlatformGetBusiness(ctx, id)
			if err != nil {
				return nil, err
			}
			counts, e := q.PlatformBusinessCounts(ctx, id)
			if e != nil {
				return nil, e
			}
			result = map[string]any{"business": b, "outletCount": counts.Outlets, "activeAdminCount": counts.ActiveAdmins}
		} else {
			items, err := q.PlatformListBusinesses(ctx, db.PlatformListBusinessesParams{Search: search, Active: active, PageLimit: limit, PageOffset: offset})
			if err != nil {
				return nil, err
			}
			total, err := q.PlatformCountBusinesses(ctx, db.PlatformCountBusinessesParams{Search: search, Active: active})
			if err != nil {
				return nil, err
			}
			if items == nil {
				items = []db.Business{}
			}
			result = platformPage(items, total, limit, offset)
		}
		if err := platformBusinessAudit(ctx, q, a, 0, id, 0, "Platform metadata inspection", "BUSINESS_METADATA_READ", "business", id, nil); err != nil {
			return nil, err
		}
		return result, nil
	})
}
func platformPage(items any, total int64, limit, offset int32) any {
	return map[string]any{"items": items, "pagination": map[string]any{"page": offset/limit + 1, "pageSize": limit, "totalItems": total, "totalPages": (total + int64(limit) - 1) / int64(limit)}}
}
func (s *PlatformTenants) Activation(ctx context.Context, a PlatformActor, id int64, active bool, reason string) (any, error) {
	if !validReason(reason) {
		return nil, ErrPlatformInvalid
	}
	return s.platformTx(ctx, a, func(ctx context.Context, q *db.Queries, _ int64) (any, error) {
		b, err := q.PlatformLockBusiness(ctx, id)
		if err != nil {
			return nil, err
		}
		if err := q.PlatformSetBusinessActive(ctx, db.PlatformSetBusinessActiveParams{ID: id, IsActive: active}); err != nil {
			return nil, err
		}
		if !active {
			if err := q.PlatformRevokeTenantFamilies(ctx, id); err != nil {
				return nil, err
			}
			if err := q.PlatformRevokeBusinessSupport(ctx, id); err != nil {
				return nil, err
			}
		}
		if err := platformBusinessAudit(ctx, q, a, 0, id, 0, reason, "BUSINESS_ACTIVATION_CHANGED", "business", id, map[string]any{"oldIsActive": b.IsActive, "isActive": active}); err != nil {
			return nil, err
		}
		return q.PlatformGetBusiness(ctx, id)
	})
}
func validReason(reason string) bool {
	return strings.TrimSpace(reason) != "" && len([]rune(reason)) <= 500
}

type SupportRequestInput struct {
	Reason               string    `json:"reason"`
	AccessScope          string    `json:"accessScope"`
	ExpiresAt            time.Time `json:"expiresAt"`
	ConfirmationPassword string    `json:"confirmationPassword"`
}

func (s *PlatformTenants) RequestSupport(ctx context.Context, business, user int64, requestID string, in SupportRequestInput) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if !validReason(in.Reason) || (in.AccessScope != "READ_ONLY" && in.AccessScope != "READ_WRITE") || !in.ExpiresAt.After(time.Now()) || in.ExpiresAt.After(time.Now().Add(time.Hour)) {
		return nil, ErrPlatformInvalid
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	owner, err := q.SupportLockOwner(ctx, db.SupportLockOwnerParams{BusinessID: business, ID: user})
	if err != nil {
		return nil, ErrPlatformDenied
	}
	if security.VerifyPassword(owner.PasswordHash, in.ConfirmationPassword) != nil {
		return nil, ErrPlatformDenied
	}
	r, err := q.SupportCreateRequest(ctx, db.SupportCreateRequestParams{BusinessID: business, RequestedByUserID: user, Reason: strings.TrimSpace(in.Reason), AccessScope: in.AccessScope, ExpiresAt: pgtype.Timestamptz{Time: in.ExpiresAt, Valid: true}})
	if err != nil {
		return nil, err
	}
	if err := platformBusinessAudit(ctx, q, PlatformActor{RequestID: requestID}, user, business, 0, r.Reason, "SUPPORT_REQUESTED", "support_request", r.ID, map[string]any{"accessScope": r.AccessScope}); err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return r, nil
}
func (s *PlatformTenants) StartSupport(ctx context.Context, a PlatformActor, business, request int64) (any, error) {
	return s.platformTx(ctx, a, func(ctx context.Context, q *db.Queries, family int64) (any, error) {
		r, err := q.SupportLockRequest(ctx, db.SupportLockRequestParams{BusinessID: business, ID: request})
		if err != nil {
			return nil, ErrPlatformDenied
		}
		session, err := q.SupportCreateSession(ctx, db.SupportCreateSessionParams{BusinessID: business, SupportRequestID: r.ID, PlatformAdminID: a.ID, PlatformFamilyID: family, Reason: r.Reason, AccessScope: r.AccessScope, ExpiresAt: r.ExpiresAt})
		if err != nil {
			return nil, err
		}
		if err := platformBusinessAudit(ctx, q, a, 0, business, session.ID, r.Reason, "SUPPORT_STARTED", "support_session", session.ID, map[string]any{"accessScope": r.AccessScope}); err != nil {
			return nil, err
		}
		return session, nil
	})
}
func (s *PlatformTenants) SupportOperation(ctx context.Context, a PlatformActor, business, session, target int64, operation, description string, version int64, limit, offset int32) (any, error) {
	return s.platformTx(ctx, a, func(ctx context.Context, q *db.Queries, family int64) (any, error) {
		grant, err := q.SupportLockSession(ctx, db.SupportLockSessionParams{BusinessID: business, ID: session, PlatformAdminID: a.ID, PlatformFamilyID: family})
		if err != nil {
			return nil, ErrPlatformDenied
		}
		write := operation == "PERFUME_DESCRIPTION_UPDATED"
		if write && grant.AccessScope != "READ_WRITE" {
			return nil, ErrPlatformDenied
		}
		var result any
		changes := map[string]any{"accessScope": grant.AccessScope}
		targetType := "business"
		switch operation {
		case "SUPPORT_DIAGNOSTICS_READ":
			result, err = q.SupportDiagnostics(ctx, business)
		case "SUPPORT_OUTLETS_READ":
			if target > 0 {
				result, err = q.GetOutlet(ctx, db.GetOutletParams{BusinessID: business, ID: target})
			} else {
				var items []db.Outlet
				items, err = q.ListOutlets(ctx, db.ListOutletsParams{BusinessID: business, Limit: limit, Offset: offset})
				if err == nil {
					var count int64
					count, err = q.CountOutlets(ctx, business)
					if items == nil {
						items = []db.Outlet{}
					}
					result = platformPage(items, count, limit, offset)
				}
			}
			targetType = "outlet"
		case "SUPPORT_PERFUMES_READ":
			items, e := q.SupportListPerfumes(ctx, db.SupportListPerfumesParams{BusinessID: business, Limit: limit, Offset: offset})
			if e != nil {
				return nil, e
			}
			if items == nil {
				items = []db.SupportListPerfumesRow{}
			}
			count, e := q.SupportCountPerfumes(ctx, business)
			if e != nil {
				return nil, e
			}
			result = platformPage(items, count, limit, offset)
			targetType = "perfume"
		case "SUPPORT_PERFUME_READ":
			result, err = q.SupportGetPerfume(ctx, db.SupportGetPerfumeParams{BusinessID: business, ID: target})
			targetType = "perfume"
		case "PERFUME_DESCRIPTION_UPDATED":
			if version < 1 || len([]rune(description)) > 1000 {
				return nil, ErrPlatformInvalid
			}
			before, e := q.SupportGetPerfume(ctx, db.SupportGetPerfumeParams{BusinessID: business, ID: target})
			if e != nil {
				return nil, e
			}
			if before.Version != version {
				return nil, ErrPlatformConflict
			}
			result, err = q.SupportUpdatePerfumeDescription(ctx, db.SupportUpdatePerfumeDescriptionParams{BusinessID: business, ID: target, Description: pgtype.Text{String: strings.TrimSpace(description), Valid: true}, Version: version})
			if errors.Is(err, pgx.ErrNoRows) {
				return nil, ErrPlatformConflict
			}
			targetType = "perfume"
			changes["description"] = "[REDACTED]"
			changes["oldVersion"] = version
			changes["version"] = version + 1
		default:
			return nil, ErrPlatformDenied
		}
		if err != nil {
			return nil, err
		}
		if targetType == "business" && target == 0 {
			target = business
		}
		if err := platformBusinessAudit(ctx, q, a, 0, business, session, grant.Reason, operation, targetType, target, changes); err != nil {
			return nil, err
		}
		valid, err := q.SupportSessionStillValid(ctx, session)
		if err != nil {
			return nil, err
		}
		if !valid {
			return nil, ErrPlatformDenied
		}
		return result, nil
	})
}

// End/revoke need exclusive row locks, and intentionally work after expiry.
func (s *PlatformTenants) EndSupport(ctx context.Context, a PlatformActor, owner, business, id int64, request bool, reason string) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if !validReason(reason) {
		return nil, ErrPlatformInvalid
	}
	run := func(ctx context.Context, q *db.Queries, _ int64) (any, error) {
		var result any
		var err error
		session := int64(0)
		action := "SUPPORT_REQUEST_REVOKED"
		target := "support_request"
		changes := map[string]any{}
		if request {
			changes["revoked"] = true
			result, err = q.SupportRevokeRequest(ctx, db.SupportRevokeRequestParams{BusinessID: business, ID: id, RevocationReason: nullableText(reason)})
		} else {
			var ended db.PlatformSupportSession
			ended, err = q.SupportEndSession(ctx, db.SupportEndSessionParams{BusinessID: business, ID: id, EndReason: nullableText(reason)})
			result = ended
			changes["endedAt"] = ended.EndedAt
			changes["accessScope"] = ended.AccessScope
			session = ended.ID
			action = "SUPPORT_ENDED"
			target = "support_session"
		}
		if err != nil {
			return nil, err
		}
		if err := platformBusinessAudit(ctx, q, a, owner, business, session, reason, action, target, id, changes); err != nil {
			return nil, err
		}
		return result, nil
	}
	if owner == 0 {
		return s.platformTx(ctx, a, run)
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	if _, err := q.SupportLockOwner(ctx, db.SupportLockOwnerParams{BusinessID: business, ID: owner}); err != nil {
		return nil, ErrPlatformDenied
	}
	result, err := run(ctx, q, 0)
	if err != nil {
		return nil, platformDBError(err)
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}
func (s *PlatformTenants) Views(ctx context.Context, a PlatformActor, owner, business, target int64, kind string, limit, offset int32) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	run := func(ctx context.Context, q *db.Queries, _ int64) (any, error) {
		var items any
		var total int64
		var err error
		switch kind {
		case "request":
			items, err = q.SupportGetRequest(ctx, db.SupportGetRequestParams{BusinessID: business, ID: target})
			if err != nil {
				return nil, platformDBError(err)
			}
			return items, nil
		case "requests":
			var rows []db.PlatformSupportRequest
			rows, err = q.SupportListRequests(ctx, db.SupportListRequestsParams{BusinessID: business, Limit: limit, Offset: offset})
			if rows == nil {
				rows = []db.PlatformSupportRequest{}
			}
			items = rows
			if err == nil {
				total, err = q.SupportCountRequests(ctx, business)
			}
		case "sessions":
			var rows []db.PlatformSupportSession
			rows, err = q.SupportListSessions(ctx, db.SupportListSessionsParams{BusinessID: business, Limit: limit, Offset: offset})
			if rows == nil {
				rows = []db.PlatformSupportSession{}
			}
			items = rows
			if err == nil {
				total, err = q.SupportCountSessions(ctx, business)
			}
		case "audit":
			var rows []db.PlatformAuditViewRow
			rows, err = q.PlatformAuditView(ctx, db.PlatformAuditViewParams{BusinessID: nullableID(business), PageLimit: limit, PageOffset: offset})
			if rows == nil {
				rows = []db.PlatformAuditViewRow{}
			}
			items = rows
			if err == nil {
				total, err = q.PlatformAuditCount(ctx, nullableID(business))
			}
		default:
			return nil, ErrPlatformInvalid
		}
		if err != nil {
			return nil, err
		}
		if a.ID > 0 {
			if err := platformBusinessAudit(ctx, q, a, 0, business, 0, "Support administration inspection", "SUPPORT_METADATA_READ", kind, 0, nil); err != nil {
				return nil, err
			}
		}
		return platformPage(items, total, limit, offset), nil
	}
	if owner == 0 {
		return s.platformTx(ctx, a, run)
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	if _, err := q.SupportLockOwner(ctx, db.SupportLockOwnerParams{BusinessID: business, ID: owner}); err != nil {
		return nil, ErrPlatformDenied
	}
	result, err := run(ctx, q, 0)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return result, nil
}

// Tenant audit views expose safe identity/action fields, not arbitrary historical payloads.
func (s *PlatformTenants) BusinessAudit(ctx context.Context, business, user int64, role string, limit, offset int32) (any, error) {
	ctx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return nil, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	current, err := q.GetActiveUser(ctx, db.GetActiveUserParams{ID: user, BusinessID: business})
	if err != nil || current.Role != role {
		return nil, ErrPlatformDenied
	}
	if current.Role != "ADMIN" {
		permissions, err := q.ListUserPermissionCodes(ctx, db.ListUserPermissionCodesParams{BusinessID: business, UserID: user})
		if err != nil {
			return nil, err
		}
		allowed := false
		for _, code := range permissions {
			if code == "AUDIT_READ" {
				allowed = true
			}
		}
		if !allowed {
			return nil, ErrPlatformDenied
		}
	}
	rows, err := q.BusinessAuditView(ctx, db.BusinessAuditViewParams{BusinessID: business, Limit: limit, Offset: offset})
	if err != nil {
		return nil, err
	}
	if rows == nil {
		rows = []db.BusinessAuditViewRow{}
	}
	total, err := q.BusinessAuditCount(ctx, business)
	if err != nil {
		return nil, err
	}
	if err := tx.Commit(ctx); err != nil {
		return nil, err
	}
	return platformPage(rows, total, limit, offset), nil
}
