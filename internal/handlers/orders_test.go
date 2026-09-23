package handlers

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
	"github.com/satriaardiperdana-2020/launlog-api/internal/service"
)

func TestOrderErrorExplainsDueDateAndQuantity(t *testing.T) {
	tests := []struct {
		name    string
		err     error
		code    string
		message string
	}{
		{
			name:    "due date before receive time",
			err:     service.ErrOrderDueAtBeforeReceive,
			code:    "INVALID_DUE_AT",
			message: "dueAt must not be earlier than the order receive time",
		},
		{
			name:    "invalid quantity",
			err:     service.ErrInvalidOrderQuantity,
			code:    "INVALID_QUANTITY",
			message: "PIECE quantities must be whole numbers",
		},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			e := echo.New()
			response := httptest.NewRecorder()
			context := e.NewContext(httptest.NewRequest(http.MethodPost, "/orders", nil), response)
			if err := orderError(context, test.err); err != nil {
				t.Fatalf("orderError returned %v", err)
			}
			if response.Code != http.StatusBadRequest {
				t.Fatalf("status = %d, want %d", response.Code, http.StatusBadRequest)
			}
			for _, expected := range []string{test.code, test.message} {
				if !strings.Contains(response.Body.String(), expected) {
					t.Errorf("response body %q does not contain %q", response.Body.String(), expected)
				}
			}
		})
	}
}
