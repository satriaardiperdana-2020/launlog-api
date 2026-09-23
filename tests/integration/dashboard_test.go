//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
	"time"
)

func TestOutletDashboardLocalConfirmedPaymentsAndExpenses(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO user_permissions(business_id,user_id,permission_code,granted_by) VALUES($1,$2,'REPORTS_READ',$2)`, f.businessID, f.userID); err != nil {
		t.Fatal(err)
	}
	var businessDate time.Time
	var dayStart, dayEnd time.Time
	if err := f.pool.QueryRow(ctx, `WITH d AS (SELECT (now() AT TIME ZONE 'Asia/Jakarta')::date AS local_date) SELECT local_date,(local_date::timestamp AT TIME ZONE 'Asia/Jakarta'),((local_date+1)::timestamp AT TIME ZONE 'Asia/Jakarta') FROM d`).Scan(&businessDate, &dayStart, &dayEnd); err != nil {
		t.Fatal(err)
	}
	var customerID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO customers(business_id,name) VALUES($1,'Dashboard customer') RETURNING id`, f.businessID).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	var categoryID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO expense_categories(business_id,name) VALUES($1,'Dashboard category') RETURNING id`, f.businessID).Scan(&categoryID); err != nil {
		t.Fatal(err)
	}
	createOrder := func(outletID int64) int64 {
		t.Helper()
		var orderID int64
		invoice := fmt.Sprintf("DASH-%d-%d", outletID, time.Now().UnixNano())
		if err := f.pool.QueryRow(ctx, `INSERT INTO orders(business_id,outlet_id,customer_id,invoice_number,total_amount,created_by) VALUES($1,$2,$3,$4,1000000,$5) RETURNING id`, f.businessID, outletID, customerID, invoice, f.userID).Scan(&orderID); err != nil {
			t.Fatal(err)
		}
		return orderID
	}
	primaryOrderID := createOrder(f.outletID)
	insertPayment := func(orderID int64, amount int64, status string, confirmedAt *time.Time, voided bool) int64 {
		t.Helper()
		var id int64
		var voidedAt any
		var voidedBy any
		var voidReason any
		if voided {
			voidedAt, voidedBy, voidReason = dayStart.Add(time.Hour), f.userID, "test correction"
		}
		if err := f.pool.QueryRow(ctx, `INSERT INTO payments(business_id,outlet_id,order_id,amount,method,status,confirmed_at,voided_at,voided_by,void_reason,created_by)
			VALUES($1,$2,$3,$4,'CASH',$5,$6,$7,$8,$9,$10) RETURNING id`, f.businessID, f.outletID, orderID, amount, status, confirmedAt, voidedAt, voidedBy, voidReason, f.userID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	startPaymentAt := dayStart
	midPaymentAt := dayStart.Add(5 * time.Hour)
	endPaymentAt := dayEnd
	refundedFixturePaymentID := insertPayment(primaryOrderID, 2500, "CONFIRMED", &startPaymentAt, false)
	insertPayment(primaryOrderID, 1500, "CONFIRMED", &midPaymentAt, false)
	insertPayment(primaryOrderID, 1111, "CONFIRMED", ptrTime(dayStart.Add(-time.Microsecond)), false)
	insertPayment(primaryOrderID, 2222, "CONFIRMED", &endPaymentAt, false)
	insertPayment(primaryOrderID, 3333, "PENDING", nil, false)
	insertPayment(primaryOrderID, 4444, "VOIDED", &startPaymentAt, true)
	// The dashboard reports gross confirmed receipts. The reserved refund table
	// is not an approved refund workflow and is intentionally not netted here.
	if _, err := f.pool.Exec(ctx, `INSERT INTO payment_refunds(business_id,outlet_id,order_id,payment_id,amount,reason,refunded_by,refunded_at) VALUES($1,$2,$3,$4,500,'legacy dashboard fixture',$5,$6)`, f.businessID, f.outletID, primaryOrderID, refundedFixturePaymentID, f.userID, startPaymentAt.Add(time.Hour)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO expenses(business_id,outlet_id,category_id,amount,description,expense_at,created_by) VALUES
		($1,$2,$3,7000,'starts at local midnight',$4,$5),
		($1,$2,$3,3000,'within local day',$6,$5),
		($1,$2,$3,111,'prior local day',$7,$5),
		($1,$2,$3,222,'next local day',$8,$5)`, f.businessID, f.outletID, categoryID, dayStart, f.userID, dayStart.Add(5*time.Hour), dayStart.Add(-time.Microsecond), dayEnd); err != nil {
		t.Fatal(err)
	}

	var secondOutletID, emptyOutletID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO outlets(business_id,code,name,phone,address) VALUES($1,'DASH-2','Dashboard outlet 2','+628123456789','Jakarta') RETURNING id`, f.businessID).Scan(&secondOutletID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO user_outlets(business_id,user_id,outlet_id) VALUES($1,$2,$3)`, f.businessID, f.userID, secondOutletID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO outlets(business_id,code,name) VALUES($1,'DASH-EMPTY','Empty dashboard outlet') RETURNING id`, f.businessID).Scan(&emptyOutletID); err != nil {
		t.Fatal(err)
	}
	secondOrderID := createOrder(secondOutletID)
	insertPayment(secondOrderID, 9000, "CONFIRMED", &midPaymentAt, false)
	if _, err := f.pool.Exec(ctx, `INSERT INTO expenses(business_id,outlet_id,category_id,amount,description,expense_at,created_by) VALUES($1,$2,$3,5000,'other outlet',$4,$5)`, f.businessID, secondOutletID, categoryID, midPaymentAt, f.userID); err != nil {
		t.Fatal(err)
	}
	token := f.login(t).Tokens.AccessToken
	requestDashboard := func(outletID int64, accessToken string) (int, []byte) {
		t.Helper()
		return f.managementRequest(http.MethodGet, fmt.Sprintf("/outlets/%d/dashboard", outletID), nil, accessToken)
	}
	type dashboard struct {
		Outlet struct {
			ID         int64   `json:"id"`
			BusinessID int64   `json:"business_id"`
			Code       string  `json:"code"`
			Name       string  `json:"name"`
			Phone      *string `json:"phone"`
			Address    *string `json:"address"`
		} `json:"outlet"`
		BusinessDate string `json:"business_date"`
		Timezone     string `json:"timezone"`
		Payments     struct {
			ConfirmedCount       int64 `json:"confirmed_count"`
			ConfirmedTotalAmount int64 `json:"confirmed_total_amount"`
		} `json:"payments"`
		Expenses struct {
			Count       int64 `json:"count"`
			TotalAmount int64 `json:"total_amount"`
		} `json:"expenses"`
	}
	status, body := requestDashboard(f.outletID, token)
	var result dashboard
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil {
		t.Fatalf("dashboard status=%d body=%s", status, body)
	}
	if result.Outlet.ID != f.outletID || result.Outlet.BusinessID != f.businessID || result.Outlet.Code != "AUTH" || result.Timezone != "Asia/Jakarta" || result.BusinessDate != businessDate.Format("2006-01-02") {
		t.Fatalf("unexpected outlet profile/local date: %+v", result)
	}
	if result.Payments.ConfirmedCount != 2 || result.Payments.ConfirmedTotalAmount != 4000 {
		t.Fatalf("dashboard must total today's confirmed receipts only; got count=%d amount=%d", result.Payments.ConfirmedCount, result.Payments.ConfirmedTotalAmount)
	}
	if result.Expenses.Count != 2 || result.Expenses.TotalAmount != 10000 {
		t.Fatalf("dashboard must total today's expense_at values only; got count=%d amount=%d", result.Expenses.Count, result.Expenses.TotalAmount)
	}
	status, body = requestDashboard(secondOutletID, token)
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil || result.Payments.ConfirmedTotalAmount != 9000 || result.Expenses.TotalAmount != 5000 {
		t.Fatalf("assigned outlet dashboard should isolate its child totals: status=%d result=%+v body=%s", status, result, body)
	}
	status, body = requestDashboard(emptyOutletID, token)
	if status != http.StatusNotFound {
		t.Fatalf("staff must not query an unassigned outlet dashboard: status=%d body=%s", status, body)
	}
	status, body = requestDashboard(f.outletID, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous dashboard request status=%d: %s", status, body)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM user_permissions WHERE business_id=$1 AND user_id=$2 AND permission_code='REPORTS_READ'`, f.businessID, f.userID); err != nil {
		t.Fatal(err)
	}
	status, body = requestDashboard(f.outletID, token)
	if status != http.StatusForbidden {
		t.Fatalf("dashboard requires REPORTS_READ for staff: status=%d body=%s", status, body)
	}
	adminToken := f.ownerLogin(t)
	status, body = requestDashboard(emptyOutletID, adminToken)
	if status != http.StatusOK || json.Unmarshal(body, &result) != nil || result.Payments.ConfirmedCount != 0 || result.Payments.ConfirmedTotalAmount != 0 || result.Expenses.Count != 0 || result.Expenses.TotalAmount != 0 {
		t.Fatalf("empty admin dashboard should return stable zero totals: status=%d result=%+v body=%s", status, result, body)
	}
	var foreignBusinessID, foreignOutletID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO businesses(name) VALUES('Dashboard foreign tenant') RETURNING id`).Scan(&foreignBusinessID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO outlets(business_id,code,name) VALUES($1,'FOREIGN','Foreign outlet') RETURNING id`, foreignBusinessID).Scan(&foreignOutletID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = f.pool.Exec(ctx, "DELETE FROM outlets WHERE business_id=$1", foreignBusinessID)
		_, _ = f.pool.Exec(ctx, "DELETE FROM businesses WHERE id=$1", foreignBusinessID)
	}()
	status, body = requestDashboard(foreignOutletID, adminToken)
	if status != http.StatusNotFound {
		t.Fatalf("admin must remain tenant-scoped: status=%d body=%s", status, body)
	}
}

func ptrTime(value time.Time) *time.Time { return &value }
