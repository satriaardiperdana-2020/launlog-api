//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
)

func TestExpenseLifecycleDateScopeConcurrencyAndRedactedAudit(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	for _, code := range []string{"EXPENSES_READ", "EXPENSES_WRITE"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO user_permissions(business_id,user_id,permission_code,granted_by) VALUES($1,$2,$3,$2)`, f.businessID, f.userID, code); err != nil {
			t.Fatal(err)
		}
	}
	token := f.login(t).Tokens.AccessToken
	status, body := f.managementRequest(http.MethodPost, "/expense-categories", map[string]any{"name": "Utilities", "description": "monthly bills"}, token)
	if status != http.StatusCreated {
		t.Fatalf("create category status=%d: %s", status, body)
	}
	var category struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &category); err != nil || category.ID < 1 {
		t.Fatalf("decode category: %+v, %v", category, err)
	}

	localNow := time.Now().In(time.FixedZone("Jakarta", 7*60*60))
	previousDay := time.Date(localNow.Year(), localNow.Month(), localNow.Day()-1, 12, 0, 0, 0, localNow.Location())
	status, body = f.managementRequest(http.MethodPost, "/expenses", map[string]any{
		"outletId": f.outletID, "categoryId": category.ID, "amount": 125000, "description": "private narrative must not enter audit",
		"expenseAt": previousDay.Format(time.RFC3339), "receiptReference": "private-receipt-123",
	}, token)
	if status != http.StatusCreated {
		t.Fatalf("create expense status=%d: %s", status, body)
	}
	var created struct {
		ID, Version int64
		ExpenseAt   time.Time `json:"expense_at"`
	}
	if err := json.Unmarshal(body, &created); err != nil || created.ID < 1 || created.Version != 1 {
		t.Fatalf("decode expense: %+v, %v", created, err)
	}

	status, body = f.managementRequest(http.MethodGet, "/expenses?page=1&pageSize=20", nil, token)
	if status != http.StatusOK {
		t.Fatalf("today default list status=%d: %s", status, body)
	}
	var defaultList struct {
		Items    []json.RawMessage `json:"items"`
		DateFrom string            `json:"date_from"`
		Timezone string            `json:"timezone"`
	}
	if err := json.Unmarshal(body, &defaultList); err != nil || len(defaultList.Items) != 0 || defaultList.Timezone != "Asia/Jakarta" || defaultList.DateFrom != localNow.Format("2006-01-02") {
		t.Fatalf("default filter must be Jakarta today and use expense_at: %+v err=%v body=%s", defaultList, err, body)
	}
	previousDate := previousDay.Format("2006-01-02")
	status, body = f.managementRequest(http.MethodGet, "/expenses?fromDate="+previousDate+"&toDate="+previousDate, nil, token)
	if status != http.StatusOK {
		t.Fatalf("historical expense filter status=%d: %s", status, body)
	}
	if err := json.Unmarshal(body, &defaultList); err != nil || len(defaultList.Items) != 1 {
		t.Fatalf("explicit date range must include expense by expense_at: count=%d err=%v body=%s", len(defaultList.Items), err, body)
	}

	getReq := httptest.NewRequest(http.MethodGet, fmt.Sprintf("/expenses/%d", created.ID), nil)
	getReq.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
	getRec := httptest.NewRecorder()
	f.echo.ServeHTTP(getRec, getReq)
	if getRec.Code != http.StatusOK || getRec.Header().Get("ETag") != "\"1\"" {
		t.Fatalf("detail must return initial ETag: status=%d etag=%q body=%s", getRec.Code, getRec.Header().Get("ETag"), getRec.Body.String())
	}
	update := map[string]any{"categoryId": category.ID, "amount": 130000, "description": "updated private narrative", "expenseAt": previousDay.Add(time.Hour).Format(time.RFC3339), "receiptReference": "changed-private-ref"}
	put := func(etag, wantETag string) (int, []byte) {
		b, _ := json.Marshal(update)
		req := httptest.NewRequest(http.MethodPut, fmt.Sprintf("/expenses/%d", created.ID), bytesReader(b))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
		if etag != "" {
			req.Header.Set("If-Match", etag)
		}
		rec := httptest.NewRecorder()
		f.echo.ServeHTTP(rec, req)
		if rec.Code == http.StatusOK && rec.Header().Get("ETag") != wantETag {
			t.Errorf("updated ETag=%q, want %s", rec.Header().Get("ETag"), wantETag)
		}
		return rec.Code, rec.Body.Bytes()
	}
	status, body = put(`"1"`, `"2"`)
	if status != http.StatusOK {
		t.Fatalf("expense update status=%d: %s", status, body)
	}
	results := make(chan int, 2)
	var updates sync.WaitGroup
	for i := 0; i < 2; i++ {
		updates.Add(1)
		go func() { defer updates.Done(); code, _ := put(`"2"`, `"3"`); results <- code }()
	}
	updates.Wait()
	close(results)
	wins, losses := 0, 0
	for code := range results {
		if code == http.StatusOK {
			wins++
		} else if code == http.StatusConflict {
			losses++
		} else {
			t.Errorf("concurrent update unexpected status=%d", code)
		}
	}
	if wins != 1 || losses != 1 {
		t.Fatalf("optimistic concurrency should allow exactly one writer, got wins=%d stale=%d", wins, losses)
	}
	status, body = put(`"2"`, `""`)
	if status != http.StatusConflict {
		t.Fatalf("stale update must not overwrite newer version: status=%d body=%s", status, body)
	}
	status, body = put("", "")
	if status != http.StatusPreconditionRequired {
		t.Fatalf("missing If-Match status=%d: %s", status, body)
	}

	var auditCount int
	var auditValues string
	if err := f.pool.QueryRow(ctx, `SELECT count(*), string_agg(COALESCE(old_values::text,'') || COALESCE(new_values::text,''),'') FROM audit_logs WHERE business_id=$1 AND entity_type='expense' AND entity_id=$2`, f.businessID, created.ID).Scan(&auditCount, &auditValues); err != nil {
		t.Fatal(err)
	}
	if auditCount != 3 || containsAny(auditValues, "private narrative", "private-receipt-123", "changed-private-ref") {
		t.Fatalf("audit records must be transactional and redact descriptions/receipt refs: count=%d audit=%s", auditCount, auditValues)
	}

	var unassignedOutlet int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO outlets(business_id,code,name) VALUES($1,'EXP-OTHER','Other outlet') RETURNING id`, f.businessID).Scan(&unassignedOutlet); err != nil {
		t.Fatal(err)
	}
	status, body = f.managementRequest(http.MethodPost, "/expenses", map[string]any{"outletId": unassignedOutlet, "categoryId": category.ID, "amount": 1, "description": "should be denied"}, token)
	if status != http.StatusNotFound {
		t.Fatalf("staff must not create expenses for an unassigned outlet: status=%d body=%s", status, body)
	}
	status, body = f.managementRequest(http.MethodPost, "/expenses", map[string]any{"outletId": f.outletID, "categoryId": category.ID, "amount": 1, "description": "unauthorized"}, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous expense create status=%d: %s", status, body)
	}
	status, body = f.managementRequest(http.MethodPut, fmt.Sprintf("/expense-categories/%d", category.ID), map[string]any{"name": "Utilities", "description": "private category detail", "isActive": false}, token)
	if status != http.StatusOK {
		t.Fatalf("category deactivation status=%d: %s", status, body)
	}
	status, body = f.managementRequest(http.MethodPost, "/expenses", map[string]any{"outletId": f.outletID, "categoryId": category.ID, "amount": 100, "description": "inactive category rejected"}, token)
	if status != http.StatusConflict {
		t.Fatalf("inactive category must not be reused: status=%d body=%s", status, body)
	}
	status, body = f.managementRequest(http.MethodGet, "/expense-categories?page=1&pageSize=20", nil, token)
	if status != http.StatusOK || strings.Contains(string(body), `"id":`+fmt.Sprint(category.ID)) {
		t.Fatalf("inactive category should be excluded by default: status=%d body=%s", status, body)
	}
	status, body = f.managementRequest(http.MethodGet, "/expense-categories?includeInactive=true&page=1&pageSize=20", nil, token)
	if status != http.StatusOK || !strings.Contains(string(body), `"id":`+fmt.Sprint(category.ID)) {
		t.Fatalf("includeInactive should expose disabled category: status=%d body=%s", status, body)
	}
	if _, err := f.pool.Exec(ctx, "DELETE FROM user_permissions WHERE business_id=$1 AND user_id=$2 AND permission_code='EXPENSES_READ'", f.businessID, f.userID); err != nil {
		t.Fatal(err)
	}
	status, body = f.managementRequest(http.MethodGet, "/expenses", nil, token)
	if status != http.StatusForbidden {
		t.Fatalf("expense listing without EXPENSES_READ must be forbidden: status=%d body=%s", status, body)
	}
}

func bytesReader(b []byte) *bytes.Reader { return bytes.NewReader(b) }
func containsAny(value string, candidates ...string) bool {
	for _, candidate := range candidates {
		if strings.Contains(value, candidate) {
			return true
		}
	}
	return false
}
