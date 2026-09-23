package handlers

import (
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/labstack/echo/v4"
)

func TestReportDateRangeValidation(t *testing.T) {
	tests := []struct {
		name string
		url  string
		want string
	}{
		{name: "one day", url: "/reports?fromDate=2026-09-21", want: ""},
		{name: "366 days", url: "/reports?fromDate=2025-09-21&toDate=2026-09-21", want: ""},
		{name: "367 days", url: "/reports?fromDate=2025-09-20&toDate=2026-09-21", want: "366"},
		{name: "reversed", url: "/reports?fromDate=2026-09-22&toDate=2026-09-21", want: "on or after"},
		{name: "invalid", url: "/reports?fromDate=2026-02-30", want: "YYYY-MM-DD"},
	}
	e := echo.New()
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			c := e.NewContext(httptest.NewRequest("GET", test.url, nil), httptest.NewRecorder())
			_, _, err := reportDateRange(c)
			if test.want == "" && err != nil {
				t.Fatalf("unexpected validation error: %v", err)
			}
			if test.want != "" && (err == nil || !strings.Contains(err.Error(), test.want)) {
				t.Fatalf("error=%v, want substring %q", err, test.want)
			}
		})
	}
}
