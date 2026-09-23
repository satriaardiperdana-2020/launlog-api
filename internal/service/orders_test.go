package service

import (
	"errors"
	"testing"
)

func TestParseMilliQuantity(t *testing.T) {
	tests := []struct {
		value string
		milli int64
		valid bool
	}{
		{value: "5", milli: 5000, valid: true},
		{value: "5.25", milli: 5250, valid: true},
		{value: "0.001", milli: 1, valid: true},
		{value: "0", valid: false},
		{value: "5.0000", valid: false},
		{value: "1e2", valid: false},
		{value: "-1", valid: false},
	}
	for _, test := range tests {
		t.Run(test.value, func(t *testing.T) {
			got, err := parseMilliQuantity(test.value)
			if test.valid {
				if err != nil || got != test.milli {
					t.Fatalf("parseMilliQuantity(%q) = %d, %v; want %d, nil", test.value, got, err, test.milli)
				}
				return
			}
			if !errors.Is(err, ErrInvalidOrderQuantity) {
				t.Fatalf("parseMilliQuantity(%q) error = %v, want ErrInvalidOrderQuantity", test.value, err)
			}
		})
	}
}

func TestAllowedOrderTransitionMatrix(t *testing.T) {
	tests := []struct {
		from, to string
		want     bool
	}{
		{"RECEIVED", "PROCESSING", true},
		{"RECEIVED", "CANCELLED", true},
		{"PROCESSING", "READY_FOR_PICKUP", true},
		{"PROCESSING", "CANCELLED", true},
		{"READY_FOR_PICKUP", "COMPLETED", true},
		{"READY_FOR_PICKUP", "CANCELLED", true},
		{"RECEIVED", "READY_FOR_PICKUP", false},
		{"RECEIVED", "COMPLETED", false},
		{"PROCESSING", "COMPLETED", false},
		{"COMPLETED", "CANCELLED", false},
		{"CANCELLED", "PROCESSING", false},
		{"CANCELLED", "CANCELLED", false},
		{"", "RECEIVED", false},
	}
	for _, tt := range tests {
		t.Run(tt.from+"_to_"+tt.to, func(t *testing.T) {
			if got := allowedOrderTransition(tt.from, tt.to); got != tt.want {
				t.Fatalf("allowedOrderTransition(%q, %q) = %v, want %v", tt.from, tt.to, got, tt.want)
			}
		})
	}
}
