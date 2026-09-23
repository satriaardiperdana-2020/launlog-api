package service

import (
	"context"
	"encoding/json"
	"errors"
	"net/netip"
	"strings"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/satriaardiperdana-2020/launlog-api/internal/repository"
	"github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
)

var (
	ErrInvalidName     = errors.New("service name is required")
	ErrInvalidUnit     = errors.New("unsupported service unit")
	ErrInvalidPrice    = errors.New("service price must be nonnegative whole rupiah")
	ErrInvalidDuration = errors.New("service duration must be nonnegative minutes")
	ErrNotFound        = errors.New("service not found")
	ErrConflict        = errors.New("service name and unit already exist")
)

type Service struct {
	ID                       int64     `json:"id"`
	BusinessID               int64     `json:"business_id"`
	Name                     string    `json:"name"`
	Description              *string   `json:"description"`
	Unit                     string    `json:"unit"`
	UnitPriceAmount          int64     `json:"unit_price_amount"`
	EstimatedDurationMinutes int32     `json:"estimated_duration_minutes"`
	IsActive                 bool      `json:"is_active"`
	CreatedAt                time.Time `json:"created_at"`
	UpdatedAt                time.Time `json:"updated_at"`
}

type ServiceInput struct {
	Name                     string
	Description              *string
	Unit                     string
	UnitPriceAmount          int64
	EstimatedDurationMinutes int32
	IsActive                 bool
}

type ServiceFilter struct {
	Search     string
	Unit       *string
	IsActive   *bool
	PageOffset int32
	PageLimit  int32
}

type Actor struct {
	BusinessID int64
	UserID     int64
	Role       string
	OutletIDs  []int64
	IP         *netip.Addr
	UserAgent  string
}

type ServiceCatalog struct{ database *repository.Postgres }

func NewServiceCatalog(database *repository.Postgres) *ServiceCatalog {
	return &ServiceCatalog{database: database}
}

func ValidUnit(unit string) bool {
	switch unit {
	case "KILOGRAM", "PIECE", "METER", "SQUARE_METER":
		return true
	default:
		return false
	}
}

func validateServiceInput(input *ServiceInput) error {
	input.Name = strings.TrimSpace(input.Name)
	if input.Name == "" {
		return ErrInvalidName
	}
	if !ValidUnit(input.Unit) {
		return ErrInvalidUnit
	}
	if input.UnitPriceAmount < 0 {
		return ErrInvalidPrice
	}
	if input.EstimatedDurationMinutes < 0 {
		return ErrInvalidDuration
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

func (s *ServiceCatalog) List(ctx context.Context, businessID int64, filter ServiceFilter) ([]Service, int64, error) {
	if filter.Unit != nil && !ValidUnit(*filter.Unit) {
		return nil, 0, ErrInvalidUnit
	}
	unit := optionalText(filter.Unit)
	active := pgtype.Bool{}
	if filter.IsActive != nil {
		active = pgtype.Bool{Bool: *filter.IsActive, Valid: true}
	}
	q := s.database.Queries()
	items, err := q.ListServices(ctx, postgresql.ListServicesParams{
		BusinessID: businessID, Search: strings.TrimSpace(filter.Search), Unit: unit,
		IsActive: active, PageOffset: filter.PageOffset, PageLimit: filter.PageLimit,
	})
	if err != nil {
		return nil, 0, err
	}
	total, err := q.CountServices(ctx, postgresql.CountServicesParams{BusinessID: businessID, Search: strings.TrimSpace(filter.Search), Unit: unit, IsActive: active})
	if err != nil {
		return nil, 0, err
	}
	result := make([]Service, 0, len(items))
	for _, item := range items {
		result = append(result, serviceFromRow(item.ID, item.BusinessID, item.Name, item.Description,
			item.Unit, item.UnitPriceAmount, item.EstimatedDurationMinutes, item.IsActive, item.CreatedAt, item.UpdatedAt))
	}
	return result, total, nil
}

func (s *ServiceCatalog) Get(ctx context.Context, businessID, serviceID int64) (Service, error) {
	item, err := s.database.Queries().GetService(ctx, postgresql.GetServiceParams{BusinessID: businessID, ServiceID: serviceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Service{}, ErrNotFound
	}
	if err != nil {
		return Service{}, err
	}
	return serviceFromRow(item.ID, item.BusinessID, item.Name, item.Description,
		item.Unit, item.UnitPriceAmount, item.EstimatedDurationMinutes, item.IsActive, item.CreatedAt, item.UpdatedAt), nil
}

func (s *ServiceCatalog) Create(ctx context.Context, actor Actor, input ServiceInput) (Service, error) {
	if err := validateServiceInput(&input); err != nil {
		return Service{}, err
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return Service{}, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	item, err := q.CreateService(ctx, postgresql.CreateServiceParams{
		BusinessID: actor.BusinessID, Name: input.Name, Description: optionalText(input.Description), Unit: input.Unit,
		UnitPriceAmount: input.UnitPriceAmount, EstimatedDurationMinutes: input.EstimatedDurationMinutes,
	})
	if err != nil {
		return Service{}, serviceWriteError(err)
	}
	created := serviceFromRow(item.ID, item.BusinessID, item.Name, item.Description,
		item.Unit, item.UnitPriceAmount, item.EstimatedDurationMinutes, item.IsActive, item.CreatedAt, item.UpdatedAt)
	if err := auditService(ctx, q, actor, "SERVICE_CREATED", item.ID, nil, created); err != nil {
		return Service{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Service{}, err
	}
	return created, nil
}

func (s *ServiceCatalog) Update(ctx context.Context, actor Actor, serviceID int64, input ServiceInput) (Service, error) {
	if err := validateServiceInput(&input); err != nil {
		return Service{}, err
	}
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return Service{}, err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	old, err := q.GetServiceForUpdate(ctx, postgresql.GetServiceForUpdateParams{BusinessID: actor.BusinessID, ServiceID: serviceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return Service{}, ErrNotFound
	}
	if err != nil {
		return Service{}, err
	}
	item, err := q.UpdateService(ctx, postgresql.UpdateServiceParams{
		BusinessID: actor.BusinessID, ServiceID: serviceID, Name: input.Name, Description: optionalText(input.Description), Unit: input.Unit,
		UnitPriceAmount: input.UnitPriceAmount, EstimatedDurationMinutes: input.EstimatedDurationMinutes, IsActive: input.IsActive,
	})
	if err != nil {
		return Service{}, serviceWriteError(err)
	}
	before := serviceFromRow(old.ID, old.BusinessID, old.Name, old.Description,
		old.Unit, old.UnitPriceAmount, old.EstimatedDurationMinutes, old.IsActive, old.CreatedAt, old.UpdatedAt)
	updated := serviceFromRow(item.ID, item.BusinessID, item.Name, item.Description,
		item.Unit, item.UnitPriceAmount, item.EstimatedDurationMinutes, item.IsActive, item.CreatedAt, item.UpdatedAt)
	action := "SERVICE_UPDATED"
	if before.IsActive && !updated.IsActive {
		action = "SERVICE_DEACTIVATED"
	} else if !before.IsActive && updated.IsActive {
		action = "SERVICE_REACTIVATED"
	}
	if err := auditService(ctx, q, actor, action, serviceID, before, updated); err != nil {
		return Service{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Service{}, err
	}
	return updated, nil
}

func (s *ServiceCatalog) Delete(ctx context.Context, actor Actor, serviceID int64) error {
	tx, err := s.database.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	q := s.database.Queries().WithTx(tx)
	old, err := q.GetServiceForUpdate(ctx, postgresql.GetServiceForUpdateParams{BusinessID: actor.BusinessID, ServiceID: serviceID})
	if errors.Is(err, pgx.ErrNoRows) {
		return ErrNotFound
	}
	if err != nil {
		return err
	}
	if _, err := q.SoftDeleteService(ctx, postgresql.SoftDeleteServiceParams{BusinessID: actor.BusinessID, ServiceID: serviceID}); err != nil {
		return err
	}
	before := serviceFromRow(old.ID, old.BusinessID, old.Name, old.Description,
		old.Unit, old.UnitPriceAmount, old.EstimatedDurationMinutes, old.IsActive, old.CreatedAt, old.UpdatedAt)
	if err := auditService(ctx, q, actor, "SERVICE_DELETED", serviceID, before, map[string]any{"deleted": true}); err != nil {
		return err
	}
	return tx.Commit(ctx)
}

func serviceFromRow(id, businessID int64, name string, description pgtype.Text, unit string, price int64,
	duration int32, active bool, createdAt, updatedAt pgtype.Timestamptz) Service {
	var text *string
	if description.Valid {
		value := description.String
		text = &value
	}
	return Service{ID: id, BusinessID: businessID, Name: name, Description: text, Unit: unit,
		UnitPriceAmount: price, EstimatedDurationMinutes: duration, IsActive: active,
		CreatedAt: createdAt.Time, UpdatedAt: updatedAt.Time}
}

func optionalText(value *string) pgtype.Text {
	if value == nil {
		return pgtype.Text{}
	}
	return pgtype.Text{String: *value, Valid: true}
}

func serviceWriteError(err error) error {
	var pgErr *pgconn.PgError
	if errors.As(err, &pgErr) && pgErr.Code == "23505" {
		return ErrConflict
	}
	return err
}

func auditService(ctx context.Context, q *postgresql.Queries, actor Actor, action string, entityID int64, before, after any) error {
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
		Action: action, EntityType: "service", EntityID: pgtype.Int8{Int64: entityID, Valid: true},
		OldValues: oldJSON, NewValues: newJSON, IpAddress: actor.IP,
		UserAgent: pgtype.Text{String: userAgent, Valid: userAgent != ""},
	})
}
