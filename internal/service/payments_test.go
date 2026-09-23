package service

import (
	"errors"
	"strings"
	"testing"
)

func TestPaymentMethodsMatchExistingDatabaseConstraint(t *testing.T) {
	for _, method := range []string{"CASH", "BCA_TRANSFER", "QRIS"} {
		if !validPaymentMethod(method) {
			t.Errorf("validPaymentMethod(%q)=false", method)
		}
	}
	for _, method := range []string{"BCA", "TRANSFER", "cash", ""} {
		if validPaymentMethod(method) {
			t.Errorf("validPaymentMethod(%q)=true", method)
		}
	}
}

func TestPaymentAmountsAndCashChange(t *testing.T) {
	tests := []struct {
		name, method      string
		tendered, balance int64
		applied, change   int64
		wantErr           error
	}{
		{name: "partial cash", method: "CASH", tendered: 4000, balance: 10000, applied: 4000},
		{name: "cash settlement with change", method: "CASH", tendered: 12000, balance: 10000, applied: 10000, change: 2000},
		{name: "exact transfer settlement", method: "BCA_TRANSFER", tendered: 10000, balance: 10000, applied: 10000},
		{name: "transfer partial", method: "QRIS", tendered: 3000, balance: 10000, applied: 3000},
		{name: "non-cash overpayment rejected", method: "QRIS", tendered: 10001, balance: 10000, wantErr: ErrPaymentOverpayment},
		{name: "zero outstanding", method: "CASH", tendered: 500, balance: 0, wantErr: ErrPaymentNoBalance},
		{name: "negative tender", method: "CASH", tendered: -1, balance: 10000, wantErr: ErrInvalidPayment},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			applied, change, err := paymentAmounts(tt.method, tt.tendered, tt.balance)
			if !errors.Is(err, tt.wantErr) {
				t.Fatalf("error=%v, want %v", err, tt.wantErr)
			}
			if applied != tt.applied || change != tt.change {
				t.Fatalf("applied=%d change=%d, want applied=%d change=%d", applied, change, tt.applied, tt.change)
			}
		})
	}
}

func TestPaymentStatusReconciliation(t *testing.T) {
	for _, tt := range []struct {
		total, paid int64
		want        string
	}{
		{10000, 0, "UNPAID"},
		{10000, 1, "PARTIALLY_PAID"},
		{10000, 10000, "PAID"},
		{0, 0, "UNPAID"},
	} {
		if got := paymentStatusFor(tt.total, tt.paid); got != tt.want {
			t.Errorf("paymentStatusFor(%d,%d)=%q want %q", tt.total, tt.paid, got, tt.want)
		}
	}
}

func TestPaymentTextLimitsCountCharacters(t *testing.T) {
	within := strings.Repeat("é", 256)
	over := strings.Repeat("é", 257)
	if exceedsPaymentTextLimit(&within, 256) {
		t.Fatal("256 Unicode characters should fit")
	}
	if !exceedsPaymentTextLimit(&over, 256) {
		t.Fatal("257 Unicode characters should exceed the limit")
	}
}
