//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/labstack/echo/v4"
	"github.com/satriaardiperdana-2020/launlog-api/internal/service"
)

func assertTimezone(t *testing.T, body []byte, field, want string) {
	t.Helper()
	var value map[string]any
	if err := json.Unmarshal(body, &value); err != nil || value[field] != want {
		t.Fatalf("want %s=%q: %s err=%v", field, want, body, err)
	}
}

func TestOutletTimezoneDefaultValidationAndTenantIsolation(t *testing.T) {
	f := newAuthFixture(t)
	token := f.ownerLogin(t)
	cases := []struct {
		name    string
		value   any
		present bool
		want    string
	}{
		{"omitted", nil, false, "Asia/Jakarta"}, {"null", nil, true, "Asia/Jakarta"},
		{"empty", "", true, "Asia/Jakarta"}, {"whitespace", " \t ", true, "Asia/Jakarta"},
		{"makassar", "Asia/Makassar", true, "Asia/Makassar"}, {"jayapura", "Asia/Jayapura", true, "Asia/Jayapura"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			in := map[string]any{"name": "Timezone outlet"}
			if tc.present {
				in["timezone"] = tc.value
			}
			status, body := f.managementRequest(http.MethodPost, "/outlets", in, token)
			if status != 201 {
				t.Fatalf("create: %d %s", status, body)
			}
			assertTimezone(t, body, "timezone", tc.want)
			var created struct {
				ID int64 `json:"id"`
			}
			if err := json.Unmarshal(body, &created); err != nil {
				t.Fatal(err)
			}
			status, body = f.managementRequest(http.MethodGet, fmt.Sprintf("/outlets/%d", created.ID), nil, token)
			if status != 200 {
				t.Fatalf("detail: %d %s", status, body)
			}
			assertTimezone(t, body, "timezone", tc.want)
			in["isActive"] = true
			status, body = f.managementRequest(http.MethodPut, fmt.Sprintf("/outlets/%d", f.outletID), in, token)
			if status != 200 {
				t.Fatalf("update: %d %s", status, body)
			}
			assertTimezone(t, body, "timezone", tc.want)
		})
	}
	for _, input := range []any{"Invalid/Timezone", "Local", 123, true, []string{"Asia/Jakarta"}} {
		for _, method := range []string{http.MethodPost, http.MethodPut} {
			path := "/outlets"
			if method == http.MethodPut {
				path = fmt.Sprintf("/outlets/%d", f.outletID)
			}
			status, body := f.managementRequest(method, path, map[string]any{"name": "Invalid", "isActive": true, "timezone": input}, token)
			if status != 400 {
				t.Fatalf("invalid %v %s: %d %s", input, method, status, body)
			}
		}
	}
	var outletCount int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM outlets WHERE business_id=$1", f.businessID).Scan(&outletCount); err != nil || outletCount != 7 {
		t.Fatalf("invalid create wrote data: count=%d err=%v", outletCount, err)
	}
	var storedZone string
	if err := f.pool.QueryRow(context.Background(), "SELECT timezone FROM outlets WHERE business_id=$1 AND id=$2", f.businessID, f.outletID).Scan(&storedZone); err != nil || storedZone != "Asia/Jayapura" {
		t.Fatalf("invalid update changed timezone: %q %v", storedZone, err)
	}

	status, body := f.managementRequest(http.MethodGet, "/outlets", nil, token)
	var page struct {
		Items []struct {
			Timezone string `json:"timezone"`
		} `json:"items"`
	}
	if status != 200 || json.Unmarshal(body, &page) != nil || len(page.Items) != 7 {
		t.Fatalf("list: %d %s", status, body)
	}
	for _, o := range page.Items {
		if o.Timezone == "" {
			t.Fatal("list missing timezone")
		}
	}
	var updates int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM audit_logs WHERE business_id=$1 AND action='OUTLET_UPDATED' AND old_values->>'timezone' IS NOT NULL AND new_values->>'timezone' IS NOT NULL", f.businessID).Scan(&updates); err != nil || updates != 6 {
		t.Fatalf("timezone audits: %d %v", updates, err)
	}
	other := newAuthFixture(t)
	otherToken := other.ownerLogin(t)
	for _, pair := range []struct {
		fixture *authFixture
		token   string
		foreign int64
	}{{f, token, other.outletID}, {other, otherToken, f.outletID}} {
		for _, method := range []string{http.MethodGet, http.MethodPut} {
			status, body := pair.fixture.managementRequest(method, fmt.Sprintf("/outlets/%d", pair.foreign), map[string]any{"name": "Cross", "isActive": true, "timezone": "Asia/Makassar"}, pair.token)
			if status != 404 {
				t.Fatalf("cross-tenant %s: %d %s", method, status, body)
			}
		}
	}
}

func TestOutletTimezoneCurrentUserAndRefresh(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, "UPDATE outlets SET timezone='Asia/Makassar' WHERE business_id=$1 AND id=$2", f.businessID, f.outletID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, "INSERT INTO outlets(business_id,code,name,timezone) VALUES($1,'UNASSIGNED','Hidden','Asia/Jayapura')", f.businessID); err != nil {
		t.Fatal(err)
	}
	var email string
	if err := f.pool.QueryRow(ctx, "SELECT email FROM users WHERE id=$1", f.userID).Scan(&email); err != nil {
		t.Fatal(err)
	}
	check := func(body []byte, nested bool, want string) sessionPayload {
		t.Helper()
		var raw map[string]json.RawMessage
		if err := json.Unmarshal(body, &raw); err != nil {
			t.Fatal(err)
		}
		if nested {
			if err := json.Unmarshal(raw["user"], &raw); err != nil {
				t.Fatal(err)
			}
		}
		var outlets []struct {
			ID       int64  `json:"id"`
			Timezone string `json:"timezone"`
		}
		if err := json.Unmarshal(raw["outlets"], &outlets); err != nil || len(outlets) != 1 || outlets[0].ID != f.outletID || outlets[0].Timezone != want {
			t.Fatalf("current user scope/timezone: %s err=%v", body, err)
		}
		var session sessionPayload
		_ = json.Unmarshal(body, &session)
		return session
	}
	status, body := f.request("/auth/login", map[string]any{"email": email, "password": "correct-password"}, "")
	if status != 200 {
		t.Fatalf("login: %d %s", status, body)
	}
	session := check(body, true, "Asia/Makassar")
	status, body = f.managementRequest(http.MethodGet, "/outlets", nil, session.Tokens.AccessToken)
	if status != 403 {
		t.Fatalf("staff list authorization widened: %d %s", status, body)
	}
	if _, err := f.pool.Exec(ctx, "UPDATE outlets SET timezone='Asia/Jayapura' WHERE business_id=$1 AND id=$2", f.businessID, f.outletID); err != nil {
		t.Fatal(err)
	}
	status, body = f.request("/auth/me", nil, session.Tokens.AccessToken)
	if status != 200 {
		t.Fatalf("me: %d %s", status, body)
	}
	check(body, false, "Asia/Jayapura")
	status, body = f.request("/auth/refresh", map[string]any{"refreshToken": session.Tokens.RefreshToken}, "")
	if status != 200 {
		t.Fatalf("refresh: %d %s", status, body)
	}
	check(body, true, "Asia/Jayapura")
}

func TestOutletTimezoneOrderInstantsAndReplay(t *testing.T) {
	f := newAuthFixture(t)
	token := f.ownerLogin(t)
	ctx := context.Background()
	var customer, svc int64
	if err := f.pool.QueryRow(ctx, "INSERT INTO customers(business_id,name) VALUES($1,'Timezone customer') RETURNING id", f.businessID).Scan(&customer); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, "INSERT INTO services(business_id,name,unit,unit_price_amount) VALUES($1,'Wash','kg',7000) RETURNING id", f.businessID).Scan(&svc); err != nil {
		t.Fatal(err)
	}
	update := func(zone string) {
		t.Helper()
		status, body := f.managementRequest(http.MethodPut, fmt.Sprintf("/outlets/%d", f.outletID), map[string]any{"name": "Auth outlet", "isActive": true, "timezone": zone}, token)
		if status != 200 {
			t.Fatalf("update outlet: %d %s", status, body)
		}
	}
	update("Asia/Makassar")
	input := map[string]any{"outletId": f.outletID, "customerId": customer, "dueAt": "2099-01-01T00:30:00+08:00", "items": []any{map[string]any{"serviceId": svc, "quantity": "1"}}}
	createCalls := 0
	create := func() (int, []byte) {
		encoded, _ := json.Marshal(input)
		req := httptest.NewRequest(http.MethodPost, "/orders", bytes.NewReader(encoded))
		req.Header.Set(echo.HeaderContentType, echo.MIMEApplicationJSON)
		req.Header.Set(echo.HeaderAuthorization, "Bearer "+token)
		req.Header.Set("Idempotency-Key", "outlet-timezone-order")
		rec := httptest.NewRecorder()
		f.echo.ServeHTTP(rec, req)
		createCalls++
		if createCalls > 1 && rec.Header().Get("Idempotency-Replayed") != "true" {
			t.Fatalf("missing replay marker: %s", rec.Body.Bytes())
		}
		return rec.Code, rec.Body.Bytes()
	}
	status, body := create()
	if status != 201 {
		t.Fatalf("create order: %d %s", status, body)
	}
	var before service.Order
	if err := json.Unmarshal(body, &before); err != nil {
		t.Fatal(err)
	}
	if before.OutletTimezone != "Asia/Makassar" || before.DueAt == nil {
		t.Fatalf("order metadata: %s", body)
	}
	var outletCodeSnapshot string
	if err := f.pool.QueryRow(ctx, "SELECT outlet_code_snapshot FROM orders WHERE business_id=$1 AND id=$2", f.businessID, before.ID).Scan(&outletCodeSnapshot); err != nil || outletCodeSnapshot != "AUTH" {
		t.Fatalf("order outlet-code snapshot=%q err=%v", outletCodeSnapshot, err)
	}
	due, _ := time.Parse(time.RFC3339, "2099-01-01T00:30:00+08:00")
	if !before.DueAt.Equal(due) {
		t.Fatal("due instant changed")
	}
	// Store an order near midnight to exercise display metadata without changing its instant.
	received, _ := time.Parse(time.RFC3339, "2026-10-09T00:30:00+09:00")
	if _, err := f.pool.Exec(ctx, "UPDATE orders SET received_at=$1 WHERE business_id=$2 AND id=$3", received, f.businessID, before.ID); err != nil {
		t.Fatal(err)
	}
	before.ReceivedAt = received
	update("Asia/Jayapura")
	assertOrder := func(body []byte) {
		t.Helper()
		var got service.Order
		if err := json.Unmarshal(body, &got); err != nil {
			t.Fatal(err)
		}
		if got.ID != before.ID || got.OutletTimezone != "Asia/Jayapura" || !got.ReceivedAt.Equal(before.ReceivedAt) || !got.CreatedAt.Equal(before.CreatedAt) || !got.UpdatedAt.Equal(before.UpdatedAt) || got.DueAt == nil || !got.DueAt.Equal(*before.DueAt) {
			t.Fatalf("order instant rewritten: %s", body)
		}
	}
	status, body = f.managementRequest(http.MethodGet, fmt.Sprintf("/orders/%d", before.ID), nil, token)
	if status != 200 {
		t.Fatalf("detail: %d %s", status, body)
	}
	assertOrder(body)
	status, body = create()
	if status != 201 {
		t.Fatalf("replay: %d %s", status, body)
	}
	assertOrder(body)
	for _, path := range []string{"/orders", fmt.Sprintf("/customers/%d/orders", customer)} {
		status, body = f.managementRequest(http.MethodGet, path, nil, token)
		if status != 200 {
			t.Fatalf("list/history: %d %s", status, body)
		}
		var page struct {
			Items []json.RawMessage `json:"items"`
		}
		if err := json.Unmarshal(body, &page); err != nil || len(page.Items) != 1 {
			t.Fatalf("list/history: %s %v", body, err)
		}
		assertTimezone(t, page.Items[0], "outlet_timezone", "Asia/Jayapura")
		var dates struct {
			Received time.Time  `json:"received_at"`
			Due      *time.Time `json:"due_at"`
		}
		if err := json.Unmarshal(page.Items[0], &dates); err != nil || !dates.Received.Equal(received) || dates.Due == nil || !dates.Due.Equal(due) {
			t.Fatalf("list/history timestamps: %s %v", body, err)
		}
	}
	for _, path := range []string{fmt.Sprintf("/outlets/%d/dashboard", f.outletID), fmt.Sprintf("/orders/%d/receipt", before.ID)} {
		status, body = f.managementRequest(http.MethodGet, path, nil, token)
		if status != 200 {
			t.Fatalf("nested outlet: %d %s", status, body)
		}
		var payload map[string]json.RawMessage
		if err := json.Unmarshal(body, &payload); err != nil {
			t.Fatal(err)
		}
		assertTimezone(t, payload["outlet"], "timezone", "Asia/Jayapura")
		if path == fmt.Sprintf("/outlets/%d/dashboard", f.outletID) {
			assertTimezone(t, body, "timezone", "Asia/Jakarta")
		}
	}

	var storedReceived, storedDue, storedCreated, storedUpdated time.Time
	if err := f.pool.QueryRow(ctx, "SELECT received_at,due_at,created_at,updated_at FROM orders WHERE business_id=$1 AND id=$2", f.businessID, before.ID).Scan(&storedReceived, &storedDue, &storedCreated, &storedUpdated); err != nil || !storedReceived.Equal(before.ReceivedAt) || !storedDue.Equal(due) || !storedCreated.Equal(before.CreatedAt) || !storedUpdated.Equal(before.UpdatedAt) {
		t.Fatalf("stored timestamps changed: %v", err)
	}
}

func TestPlatformOnboardingOutletTimezone(t *testing.T) {
	p := newPlatformTenantFixture(t)
	for i, tc := range []struct {
		value   any
		present bool
		want    string
	}{{nil, false, "Asia/Jakarta"}, {nil, true, "Asia/Jakarta"}, {"", true, "Asia/Jakarta"}, {"  ", true, "Asia/Jakarta"}, {"Asia/Makassar", true, "Asia/Makassar"}, {"Asia/Jayapura", true, "Asia/Jayapura"}} {
		input := provisionInput(fmt.Sprintf("Timezone Laundry %d", i), fmt.Sprintf("timezone-%d-%s@example.test", i, p.suffix))
		if tc.present {
			input["firstOutlet"].(map[string]any)["timezone"] = tc.value
		}
		body := p.expect(t, http.MethodPost, "/platform/businesses", input, p.token, 201, "")
		var tenant provisionedTenant
		if err := json.Unmarshal(body, &tenant); err != nil {
			t.Fatal(err)
		}
		p.businessIDs = append(p.businessIDs, tenant.Business.ID)
		var result map[string]json.RawMessage
		_ = json.Unmarshal(body, &result)
		assertTimezone(t, result["firstOutlet"], "timezone", tc.want)
	}
	var before int
	if err := p.f.pool.QueryRow(context.Background(), "SELECT count(*) FROM businesses").Scan(&before); err != nil {
		t.Fatal(err)
	}
	for _, value := range []any{"Invalid/Timezone", "Local", false} {
		input := provisionInput("Rejected", fmt.Sprintf("rejected-%s@example.test", p.suffix))
		input["firstOutlet"].(map[string]any)["timezone"] = value
		p.expect(t, http.MethodPost, "/platform/businesses", input, p.token, 400, "")
	}
	var after int
	if err := p.f.pool.QueryRow(context.Background(), "SELECT count(*) FROM businesses").Scan(&after); err != nil || after != before {
		t.Fatalf("invalid onboarding committed records: %d -> %d %v", before, after, err)
	}
}

func TestOutletTimezoneAuditFailureRollsBack(t *testing.T) {
	f := newAuthFixture(t)
	token := f.ownerLogin(t)
	ctx := context.Background()
	function := fmt.Sprintf("outlet_timezone_fail_%d", f.businessID)
	sql := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.business_id=%d AND NEW.action='OUTLET_UPDATED' THEN RAISE EXCEPTION 'timezone audit failure'; END IF; RETURN NEW; END $$; CREATE TRIGGER %s BEFORE INSERT ON audit_logs FOR EACH ROW EXECUTE FUNCTION %s();`, function, f.businessID, function, function)
	if _, err := f.adminPool.Exec(ctx, sql); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.adminPool.Exec(ctx, fmt.Sprintf("DROP TRIGGER %s ON audit_logs; DROP FUNCTION %s();", function, function)); err != nil {
			t.Error(err)
		}
	})
	status, body := f.managementRequest(http.MethodPut, fmt.Sprintf("/outlets/%d", f.outletID), map[string]any{"name": "Auth outlet", "isActive": true, "timezone": "Asia/Jayapura"}, token)
	if status != 500 {
		t.Fatalf("audit failure: %d %s", status, body)
	}
	var zone string
	if err := f.pool.QueryRow(ctx, "SELECT timezone FROM outlets WHERE business_id=$1 AND id=$2", f.businessID, f.outletID).Scan(&zone); err != nil || zone != "Asia/Jakarta" {
		t.Fatalf("partial timezone update: %q %v", zone, err)
	}
}
