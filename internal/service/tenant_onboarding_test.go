package service

import (
	"errors"
	"strings"
	"testing"

	"github.com/satriaardiperdana-2020/launlog-api/internal/timezone"
)

func validProvisionInput() ProvisionBusinessInput {
	return ProvisionBusinessInput{
		Business:    BusinessInput{Name: " Laundry "},
		FirstOutlet: FirstOutletInput{Name: " Outlet ", Timezone: nil},
		FirstAdmin:  FirstAdminInput{Email: " Owner@Example.Test ", FullName: " Owner ", Password: "correct-password"},
	}
}

func TestNormalizeProvisionInput(t *testing.T) {
	in, err := normalizeProvisionInput(validProvisionInput())
	if err != nil {
		t.Fatal(err)
	}
	if in.FirstAdmin.Email != "owner@example.test" || in.FirstAdmin.FullName != "Owner" || in.Business.Name != "Laundry" || in.FirstOutlet.Name != "Outlet" || *in.FirstOutlet.Timezone != "Asia/Jakarta" {
		t.Fatalf("unexpected normalized provision input: %+v", in)
	}

	for _, test := range []struct {
		name, password string
		wantErr        error
	}{
		{name: "bcrypt maximum bytes", password: strings.Repeat("é", 36)},
		{name: "over bcrypt maximum bytes", password: strings.Repeat("é", 37), wantErr: ErrInvalidOwnerPassword},
		{name: "below character minimum", password: "short", wantErr: ErrInvalidOwnerPassword},
	} {
		t.Run(test.name, func(t *testing.T) {
			input := validProvisionInput()
			input.FirstAdmin.Password = test.password
			normalized, err := normalizeProvisionInput(input)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
			if err == nil && normalized.FirstAdmin.Password != test.password {
				t.Fatal("password was changed or truncated")
			}
		})
	}
}

func TestNormalizeProvisionInputRejectsInvalidFieldsAndTimezone(t *testing.T) {
	tests := []struct {
		name    string
		update  func(*ProvisionBusinessInput)
		wantErr error
	}{
		{name: "invalid email", update: func(in *ProvisionBusinessInput) { in.FirstAdmin.Email = "not-an-email" }, wantErr: ErrInvalidOwnerInput},
		{name: "business name with NUL", update: func(in *ProvisionBusinessInput) { in.Business.Name = "Laundry\x00" }, wantErr: ErrInvalidOwnerInput},
		{name: "outlet phone with NUL", update: func(in *ProvisionBusinessInput) { value := "0812\x00"; in.FirstOutlet.Phone = &value }, wantErr: ErrInvalidOwnerInput},
		{name: "missing full name", update: func(in *ProvisionBusinessInput) { in.FirstAdmin.FullName = "  " }, wantErr: ErrInvalidOwnerInput},
		{name: "invalid timezone", update: func(in *ProvisionBusinessInput) { zone := "Invalid/Zone"; in.FirstOutlet.Timezone = &zone }, wantErr: timezone.ErrInvalid},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			in := validProvisionInput()
			test.update(&in)
			_, err := normalizeProvisionInput(in)
			if !errors.Is(err, test.wantErr) {
				t.Fatalf("error = %v, want %v", err, test.wantErr)
			}
		})
	}
}
