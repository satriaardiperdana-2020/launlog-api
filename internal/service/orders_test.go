package service

import "testing"

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
