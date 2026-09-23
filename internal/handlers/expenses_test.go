package handlers

import (
	"fmt"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestExpenseETagParsing(t *testing.T) {
	for _, tc := range []struct {
		value string
		want  int64
		ok    bool
	}{
		{`"1"`, 1, true}, {`"42"`, 42, true}, {"", 0, false}, {"1", 0, false}, {`W/"1"`, 0, false}, {`"0"`, 0, false}, {`"-1"`, 0, false},
	} {
		got, err := parseExpenseETag(tc.value)
		if (err == nil) != tc.ok || got != tc.want {
			t.Errorf("parseExpenseETag(%q)=(%d,%v), want (%d, ok=%t)", tc.value, got, err, tc.want, tc.ok)
		}
	}
}

func TestExpenseDateRangeDefaultsAndValidatesJakartaDates(t *testing.T) {
	e := echo.New()
	req := httptest.NewRequest("GET", "/expenses", nil)
	rec := httptest.NewRecorder()
	c := e.NewContext(req, rec)
	from, to, err := expenseDateRange(c)
	if err != nil {
		t.Fatal(err)
	}
	loc, _ := time.LoadLocation("Asia/Jakarta")
	today := time.Now().In(loc)
	if from.Format("2006-01-02") != today.Format("2006-01-02") || to.Format("2006-01-02") != today.Format("2006-01-02") {
		t.Fatalf("default range=%s..%s, expected local today", from, to)
	}
	req = httptest.NewRequest("GET", "/expenses?fromDate=2026-09-21", nil)
	c = e.NewContext(req, httptest.NewRecorder())
	from, to, err = expenseDateRange(c)
	if err != nil || from.Format("2006-01-02") != "2026-09-21" || to.Format("2006-01-02") != "2026-09-21" {
		t.Fatalf("single date filter should select one local day: %v..%v err=%v", from, to, err)
	}
	req = httptest.NewRequest("GET", "/expenses?fromDate=2026-09-22&toDate=2026-09-21", nil)
	c = e.NewContext(req, httptest.NewRecorder())
	if _, _, err := expenseDateRange(c); err == nil || !strings.Contains(err.Error(), "on or after") {
		t.Fatalf("reversed range should fail, got %v", err)
	}
	req = httptest.NewRequest("GET", "/expenses?fromDate=2026-02-30", nil)
	c = e.NewContext(req, httptest.NewRecorder())
	if _, _, err := expenseDateRange(c); err == nil {
		t.Fatal("invalid calendar date should fail")
	}
}

func TestExpenseAuditAllowlistRedactsNarrativeAndReceipt(t *testing.T) {
	values := expenseAuditValues(expenseResponse{ID: 5, OutletID: 6, CategoryID: 7, Amount: 89000, Description: "contains name and phone", ReceiptReference: stringPointer("private-ref"), Version: 2})
	encoded := fmt.Sprint(values)
	if strings.Contains(encoded, "contains name") || strings.Contains(encoded, "private-ref") {
		t.Fatalf("audit projection leaked private expense text: %s", encoded)
	}
	if values["description"] != "[REDACTED]" || values["receipt_reference"] != "[REDACTED]" {
		t.Fatalf("audit fields are not explicitly redacted: %#v", values)
	}
}

func TestExpenseAuditNilMeansSQLNullNotJSONNull(t *testing.T) {
	value, err := marshalExpenseAuditValue(nil)
	if err != nil || value != nil {
		t.Fatalf("nil audit side must be stored as SQL NULL, got %q err=%v", value, err)
	}
}

func stringPointer(v string) *string { return &v }
