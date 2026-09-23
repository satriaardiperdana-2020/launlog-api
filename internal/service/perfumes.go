package service

import (
	"context"
	"encoding/json"
	"errors"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
)

var (
	ErrInvalidPerfumeName = errors.New("perfume name is required")
	ErrPerfumeNotFound    = errors.New("perfume not found")
	ErrPerfumeConflict    = errors.New("perfume name already exists")
)

type Perfume struct {
	ID          int64     `json:"id"`
	BusinessID  int64     `json:"business_id"`
	Name        string    `json:"name"`
	Description *string   `json:"description"`
	IsActive    bool      `json:"is_active"`
	CreatedAt   time.Time `json:"created_at"`
	UpdatedAt   time.Time `json:"updated_at"`
}

type PerfumeInput struct {
	Name        string
	Description *string
	IsActive    bool
}

type PerfumeFilter struct {
	Search     string
	IsActive   *bool
	PageOffset int32
	PageLimit  int32
}

type PerfumeCatalog struct{ database *repository.Postgres }

func NewPerfumeCatalog(database *repository.Postgres) *PerfumeCatalog {
	return &PerfumeCatalog{database: database}
}

func normalizePerfume(input *PerfumeInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return ErrInvalidPerfumeName
	}
	if input.Description != nil {
		value := strings.TrimSpace(*input.Description)
		if value == "" {
			input.Description = nil
		} else {
			input.Description = &value
		}
	}
	return nil
}

func (s *PerfumeCatalog) List(ctx context.Context, businessID int64, filter PerfumeFilter) ([]Perfume, int64, error) {
	active := pgtype.Bool{}
	if filter.IsActive != nil {
		active = pgtype.Bool{Bool: *filter.IsActive, Valid: true}
	}
	q := s.database.Queries()
	search := strings.TrimSpace(filter.Search)
	items, err := q.ListPerfumes(ctx, postgresql.ListPerfumesParams{
		BusinessID: businessID, Search: search, IsActive: active, PageOffset: filter.PageOffset, PageLimit: filter.PageLimit,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountPerfumes(ctx, postgresql.CountPerfumesParams{BusinessID: businessID, Search: search, IsActive: active})
	if err != nil {
		return nil, 0, err
	}
	result := make([]Perfume, 0, len(items))
	for _, item := range items {
		result = append(result, perfumeFromRow(item.ID, item.BusinessID, item.Name, item.Description, item.IsActive, item.CreatedAt, item.UpdatedAt))
	}
	return result, total, nil
}

func (s *PerfumeCatalog) Get(ctx context.Context, businessID, perfumeID int64) (Perfume, error) {
	item, err := s.database.Queries().GetPerfume(ctx, postgresql.GetPerfumeParams{BusinessID: businessID, PerfumeID: perfumeID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Perfume{}, ErrPerfumeNotFound
	}
	if err != nil {
		return Perfume{}, err
	}
	return perfumeFromRow(item.ID, item.BusinessID, item.Name, item.Description, item.IsActive, item.CreatedAt, item.UpdatedAt), nil
}

func (s *PerfumeCatalog) Create(ctx context.Context, actor Actor, input PerfumeInput) (Perfume, error) {
	if err := normalizePerfume(&input); err != nil {
		return Perfume{}, err
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return Perfume{}, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	item, err := q.CreatePerfume(ctx, postgresql.CreatePerfumeParams{
		BusinessID: actor.BusinessID, Name: input.Name, Description: optionalText(input.Description),
	})
	if err != nil {
		return Perfume{}, perfumeWriteError(err)
	}
	created := perfumeFromRow(item.ID, item.BusinessID, item.Name, item.Description, item.IsActive, item.CreatedAt, item.UpdatedAt)
	if err := auditPerfume(ctx, q, actor, "PERFUME_CREATED", item.ID, nil, created); err != nil {
		return Perfume{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Perfume{}, err
	}
	return created, nil
}

func (s *PerfumeCatalog) Update(ctx context.Context, actor Actor, perfumeID int64, input PerfumeInput) (Perfume, error) {
	if err := normalizePerfume(&input); err != nil {
		return Perfume{}, err
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return Perfume{}, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	old, err := q.GetPerfumeForUpdate(ctx, postgresql.GetPerfumeForUpdateParams{BusinessID: actor.BusinessID, PerfumeID: perfumeID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Perfume{}, ErrPerfumeNotFound
	}
	if err != nil {
		return Perfume{}, err
	}
	item, err := q.UpdatePerfume(ctx, postgresql.UpdatePerfumeParams{
		BusinessID: actor.BusinessID, PerfumeID: perfumeID, Name: input.Name,
		Description: optionalText(input.Description), IsActive: input.IsActive,
	})
	if err != nil {
		return Perfume{}, perfumeWriteError(err)
	}
	before := perfumeFromRow(old.ID, old.BusinessID, old.Name, old.Description, old.IsActive, old.CreatedAt, old.UpdatedAt)
	updated := perfumeFromRow(item.ID, item.BusinessID, item.Name, item.Description, item.IsActive, item.CreatedAt, item.UpdatedAt)
	action := "PERFUME_UPDATED"
	if before.IsActive && !updated.IsActive {
		action = "PERFUME_DEACTIVATED"
	} else if !before.IsActive && updated.IsActive {
		action = "PERFUME_REACTIVATED"
	}
	if err := auditPerfume(ctx, q, actor, action, perfumeID, before, updated); err != nil {
		return Perfume{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Perfume{}, err
	}
	return updated, nil
}

func (s *PerfumeCatalog) Delete(ctx context.Context, actor Actor, perfumeID int64) error {
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	old, err := q.GetPerfumeForUpdate(ctx, postgresql.GetPerfumeForUpdateParams{BusinessID: actor.BusinessID, PerfumeID: perfumeID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrPerfumeNotFound
	}
	if err != nil {
		return err
	}
	if _, err := q.SoftDeletePerfume(ctx, postgresql.SoftDeletePerfumeParams{BusinessID: actor.BusinessID, PerfumeID: perfumeID}); err != nil {
		return err
	}
	before := perfumeFromRow(old.ID, old.BusinessID, old.Name, old.Description, old.IsActive, old.CreatedAt, old.UpdatedAt)
	if err := auditPerfume(ctx, q, actor, "PERFUME_DELETED", perfumeID, before, map[string]any{"deleted": true}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func perfumeFromRow(id, businessID int64, name string, description pgtype.Text, active bool, createdAt, updatedAt pgtype.Timestamptz) Perfume {
	var text *string
	if description.Valid {
		value := description.String
		text = &value
	}
	return Perfume{ID: id, BusinessID: businessID, Name: name, Description: text, IsActive: active, CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time}
}

func perfumeWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrPerfumeConflict
	}
	return err
}

func auditPerfume(ctx context.Context, q *postgresql.Queries, actor Actor, action string, entityID int64, before, after any) error {
	var oldJSON, newJSON []byte
	var err error
	if before != nil {
		oldJSON, err = json.Marshal(before)
		if err != nil {
			return err
		}
	}
	if after != nil {
		newJSON, err = json.Marshal(after)
		if err != nil {
			return err
		}
	}
	userAgent := actor.UserAgent
	if len(userAgent) > 512 {
		userAgent = userAgent[:512]
	}
	return q.InsertAuditLog(ctx, postgresql.InsertAuditLogParams{
		BusinessID: actor.BusinessID, ActorUserID: pgtype.Int8{Int64: actor.UserID, Valid: true},
		Action: action, EntityType: "perfume", EntityID: pgtype.Int8{Int64: entityID, Valid: true},
		OldValues: oldJSON, NewValues: newJSON, IpAddress: actor.IP,
		UserAgent: pgtype.Text{String: userAgent, Valid: userAgent != ""},
	})
}
