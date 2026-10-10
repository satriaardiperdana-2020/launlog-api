package service

import (
	"context"
	"errors"
	"net/mail"
	"strings"

	"github.com/jackc/pgx/v5/pgtype"
	db "github.com/satriaardiperdana-2020/launlog-api/internal/repository/postgresql"
	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
	"github.com/satriaardiperdana-2020/launlog-api/internal/timezone"
)

var (
	ErrInvalidOwnerInput     = errors.New("invalid owner registration input")
	ErrInvalidOwnerPassword  = errors.New("invalid owner password")
	ErrOwnerEmailUnavailable = errors.New("owner email unavailable")
)

type BusinessInput struct {
	Name    string  `json:"name"`
	Phone   *string `json:"phone"`
	Address *string `json:"address"`
}

type FirstOutletInput struct {
	Name     string  `json:"name"`
	Timezone *string `json:"timezone"`
	Phone    *string `json:"phone"`
	Address  *string `json:"address"`
}

type FirstAdminInput struct {
	Email    string `json:"email"`
	FullName string `json:"fullName"`
	Password string `json:"password"`
}

type ProvisionBusinessInput struct {
	Business    BusinessInput    `json:"business"`
	FirstOutlet FirstOutletInput `json:"firstOutlet"`
	FirstAdmin  FirstAdminInput  `json:"firstAdmin"`
}

func validText(s string, max int) bool {
	return !strings.ContainsRune(s, '\x00') && len([]rune(s)) <= max
}

func validOptional(s *string, max int) bool { return s == nil || validText(*s, max) }

// Shared validation preserves the platform provisioning contract. Authority is
// absent from these input types; the insert query fixes the first role to ADMIN.
func normalizeProvisionInput(in ProvisionBusinessInput) (ProvisionBusinessInput, error) {
	in.Business.Name = strings.TrimSpace(in.Business.Name)
	in.FirstOutlet.Name = strings.TrimSpace(in.FirstOutlet.Name)
	in.FirstAdmin.Email = strings.ToLower(strings.TrimSpace(in.FirstAdmin.Email))
	in.FirstAdmin.FullName = strings.TrimSpace(in.FirstAdmin.FullName)
	zone, err := timezone.Normalize(in.FirstOutlet.Timezone)
	if err != nil {
		return in, err
	}
	in.FirstOutlet.Timezone = &zone
	email, err := mail.ParseAddress(in.FirstAdmin.Email)
	if err != nil || email.Address != in.FirstAdmin.Email || !validText(in.FirstAdmin.Email, 254) || in.Business.Name == "" || !validText(in.Business.Name, 200) || in.FirstOutlet.Name == "" || !validText(in.FirstOutlet.Name, 200) || in.FirstAdmin.FullName == "" || !validText(in.FirstAdmin.FullName, 200) || !validOptional(in.Business.Phone, 32) || !validOptional(in.Business.Address, 500) || !validOptional(in.FirstOutlet.Phone, 32) || !validOptional(in.FirstOutlet.Address, 500) {
		return in, ErrInvalidOwnerInput
	}
	if err := security.ValidateNewPassword(in.FirstAdmin.Password); err != nil {
		return in, ErrInvalidOwnerPassword
	}
	return in, nil
}

type provisionedBusiness struct {
	Business    db.Business
	FirstOutlet db.CreateOutletRow
	FirstAdmin  db.PlatformCreateFirstAdminRow
}

// The caller owns the transaction and audit policy for platform or public entry.
func createTenant(ctx context.Context, q *db.Queries, in ProvisionBusinessInput, hash string) (provisionedBusiness, error) {
	var result provisionedBusiness
	b, err := q.PlatformCreateBusiness(ctx, db.PlatformCreateBusinessParams{Name: in.Business.Name, Phone: optionalPlatformText(in.Business.Phone), Address: optionalPlatformText(in.Business.Address)})
	if err != nil {
		return result, err
	}
	code, err := q.AllocateOutletCode(ctx, b.ID)
	if err != nil {
		return result, err
	}
	o, err := q.CreateOutlet(ctx, db.CreateOutletParams{BusinessID: b.ID, Code: code, Name: in.FirstOutlet.Name, Phone: optionalPlatformText(in.FirstOutlet.Phone), Address: optionalPlatformText(in.FirstOutlet.Address), Timezone: pgtype.Text{String: *in.FirstOutlet.Timezone, Valid: true}})
	if err != nil {
		return result, err
	}
	u, err := q.PlatformCreateFirstAdmin(ctx, db.PlatformCreateFirstAdminParams{BusinessID: b.ID, Email: in.FirstAdmin.Email, FullName: in.FirstAdmin.FullName, PasswordHash: hash})
	if err != nil {
		return result, err
	}
	if err := q.AddStaffOutlet(ctx, db.AddStaffOutletParams{BusinessID: b.ID, UserID: u.ID, OutletID: o.ID}); err != nil {
		return result, err
	}
	return provisionedBusiness{Business: b, FirstOutlet: o, FirstAdmin: u}, nil
}
