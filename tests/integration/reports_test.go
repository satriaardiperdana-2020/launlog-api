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

type reportPayload struct {
	FromDate     string           `json:"from_date"`
	ToDate       string           `json:"to_date"`
	Timezone     string           `json:"timezone"`
	Summary      map[string]int64 `json:"summary"`
	Daily        []map[string]any `json:"daily"`
	ByMethod     []map[string]any `json:"by_method"`
	TopCustomers []map[string]any `json:"top_customers"`
}

func TestOutletReportsHandCalculatedReconciliation(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	if _, err := f.pool.Exec(ctx, `INSERT INTO user_permissions(business_id,user_id,permission_code,granted_by) VALUES($1,$2,'REPORTS_READ',$2)`, f.businessID, f.userID); err != nil {
		t.Fatal(err)
	}
	loc, err := time.LoadLocation("Asia/Jakarta")
	if err != nil {
		t.Fatal(err)
	}
	day1 := time.Date(2026, time.September, 21, 0, 0, 0, 0, loc)
	day2 := day1.AddDate(0, 0, 1)
	day3 := day1.AddDate(0, 0, 2)
	var customers []int64
	for i := 1; i <= 12; i++ {
		var id int64
		name := fmt.Sprintf("Customer %02d", i)
		if err := f.pool.QueryRow(ctx, `INSERT INTO customers(business_id,name) VALUES($1,$2) RETURNING id`, f.businessID, name).Scan(&id); err != nil {
			t.Fatal(err)
		}
		customers = append(customers, id)
	}
	createOrder := func(customerID int64, amount int64, receivedAt time.Time, cancelledAt *time.Time) int64 {
		t.Helper()
		invoice := fmt.Sprintf("REPORT-%d-%d", customerID, receivedAt.UnixNano())
		var id int64
		var err error
		if cancelledAt == nil {
			err = f.pool.QueryRow(ctx, `INSERT INTO orders(business_id,outlet_id,customer_id,invoice_number,total_amount,received_at,created_by)
				VALUES($1,$2,$3,$4,$5,$6,$7) RETURNING id`, f.businessID, f.outletID, customerID, invoice, amount, receivedAt, f.userID).Scan(&id)
		} else {
			err = f.pool.QueryRow(ctx, `INSERT INTO orders(business_id,outlet_id,customer_id,invoice_number,status,total_amount,received_at,cancelled_at,cancelled_by,cancellation_reason,deleted_at,created_by)
				VALUES($1,$2,$3,$4,'CANCELLED',$5,$6,$7,$8,'fixture cancellation',$7,$8) RETURNING id`, f.businessID, f.outletID, customerID, invoice, amount, receivedAt, *cancelledAt, f.userID).Scan(&id)
		}
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	orderIDs := make([]int64, len(customers))
	for i, customerID := range customers {
		var cancelledAt *time.Time
		if i == 11 {
			cancelledAt = &day2
		}
		orderIDs[i] = createOrder(customerID, 1000, day1.Add(time.Hour), cancelledAt)
	}
	secondOrderID := createOrder(customers[0], 500, day2.Add(time.Hour), nil)
	insertPayment := func(orderID, amount int64, method, status string, confirmedAt *time.Time, voided bool) int64 {
		t.Helper()
		var id int64
		var voidedAt, voidedBy, reason any
		if voided {
			voidedAt, voidedBy, reason = day2, f.userID, "fixture void"
		}
		err := f.pool.QueryRow(ctx, `INSERT INTO payments(business_id,outlet_id,order_id,amount,method,status,confirmed_at,voided_at,voided_by,void_reason,created_by)
			VALUES($1,$2,$3,$4,$5,$6,$7,$8,$9,$10,$11) RETURNING id`, f.businessID, f.outletID, orderID, amount, method, status, confirmedAt, voidedAt, voidedBy, reason, f.userID).Scan(&id)
		if err != nil {
			t.Fatal(err)
		}
		return id
	}
	confirmedID := insertPayment(orderIDs[0], 500, "CASH", "CONFIRMED", &day1, false)
	insertPayment(orderIDs[1], 1000, "QRIS", "CONFIRMED", ptrTime(day1.Add(time.Hour)), false)
	insertPayment(secondOrderID, 500, "CASH", "CONFIRMED", ptrTime(day2), false)
	insertPayment(orderIDs[2], 9000, "CASH", "VOIDED", nil, true)
	insertPayment(orderIDs[3], 700, "BCA_TRANSFER", "PENDING", nil, false)
	// Exactly at the exclusive upper boundary: included in the current order
	// cohort balance, but excluded from this payment-date income range.
	insertPayment(orderIDs[2], 100, "CASH", "CONFIRMED", &day3, false)
	if _, err := f.pool.Exec(ctx, `INSERT INTO payment_refunds(business_id,outlet_id,order_id,payment_id,amount,reason,refunded_by,refunded_at) VALUES($1,$2,$3,$4,100,'unapproved fixture row',$5,$6)`, f.businessID, f.outletID, orderIDs[0], confirmedID, f.userID, day2); err != nil {
		t.Fatal(err)
	}
	var categoryID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO expense_categories(business_id,name) VALUES($1,'Report fixture') RETURNING id`, f.businessID).Scan(&categoryID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO expenses(business_id,outlet_id,category_id,amount,description,expense_at,created_by) VALUES
		($1,$2,$3,700,'day one',$4,$5),($1,$2,$3,300,'day two',$6,$5),($1,$2,$3,900,'exclusive end boundary',$7,$5)`, f.businessID, f.outletID, categoryID, day1, f.userID, day2, day3); err != nil {
		t.Fatal(err)
	}

	token := f.login(t).Tokens.AccessToken
	getReport := func(name string) reportPayload {
		t.Helper()
		path := fmt.Sprintf("/outlets/%d/reports/%s?fromDate=2026-09-21&toDate=2026-09-22", f.outletID, name)
		status, body := f.managementRequest(http.MethodGet, path, nil, token)
		var got reportPayload
		if status != http.StatusOK || json.Unmarshal(body, &got) != nil {
			t.Fatalf("%s report status=%d body=%s", name, status, body)
		}
		if got.FromDate != "2026-09-21" || got.ToDate != "2026-09-22" || got.Timezone != "Asia/Jakarta" {
			t.Fatalf("%s report range/timezone: %+v", name, got)
		}
		return got
	}
	income := getReport("income")
	if income.Summary["payment_count"] != 3 || income.Summary["received_amount"] != 2000 || len(income.ByMethod) != 2 {
		t.Fatalf("income must include confirmed gross receipts only: %+v", income)
	}
	expenses := getReport("expenses")
	if expenses.Summary["expense_count"] != 2 || expenses.Summary["expense_amount"] != 1000 {
		t.Fatalf("expenses must use expense_at and exclude exclusive end: %+v", expenses.Summary)
	}
	profitLoss := getReport("profit-loss")
	if profitLoss.Summary["received_amount"] != 2000 || profitLoss.Summary["expense_amount"] != 1000 || profitLoss.Summary["profit_loss_amount"] != 1000 || len(profitLoss.Daily) != 2 {
		t.Fatalf("cash-basis profit/loss reconciliation mismatch: %+v", profitLoss)
	}
	orders := getReport("orders")
	if orders.Summary["order_count"] != 13 || orders.Summary["order_value_amount"] != 12500 || orders.Summary["collected_amount"] != 2100 || orders.Summary["outstanding_balance_amount"] != 9400 || orders.Summary["cancelled_count"] != 1 || orders.Summary["cancelled_order_value_amount"] != 1000 {
		t.Fatalf("order value/collection/balance must remain distinct: %+v", orders.Summary)
	}
	cancellations := getReport("cancellations")
	if cancellations.Summary["cancellation_count"] != 1 || cancellations.Summary["cancelled_order_value_amount"] != 1000 {
		t.Fatalf("retained soft-deleted cancellation missing: %+v", cancellations.Summary)
	}
	customersReport := getReport("customers")
	if customersReport.Summary["distinct_customer_count"] != 12 || len(customersReport.TopCustomers) != 10 {
		t.Fatalf("distinct customer/top-ten totals mismatch: %+v", customersReport)
	}
	if customersReport.TopCustomers[0]["name"] != "Customer 01" || customersReport.TopCustomers[1]["name"] != "Customer 02" || customersReport.TopCustomers[9]["name"] != "Customer 10" {
		t.Fatalf("top-ten tie ordering is not deterministic: %+v", customersReport.TopCustomers)
	}
	var unassignedOutletID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO outlets(business_id,code,name) VALUES($1,'REPORT-UNASSIGNED','Unassigned report outlet') RETURNING id`, f.businessID).Scan(&unassignedOutletID); err != nil {
		t.Fatal(err)
	}
	status, body := f.managementRequest(http.MethodGet, fmt.Sprintf("/outlets/%d/reports/income?fromDate=2026-09-21", unassignedOutletID), nil, token)
	if status != http.StatusNotFound {
		t.Fatalf("staff must not report on unassigned outlet: status=%d body=%s", status, body)
	}
	tooWidePath := fmt.Sprintf("/outlets/%d/reports/income?fromDate=2024-01-01&toDate=2026-09-23", f.outletID)
	status, body = f.managementRequest(http.MethodGet, tooWidePath, nil, token)
	if status != http.StatusBadRequest {
		t.Fatalf("report range above 366 days status=%d body=%s", status, body)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM user_permissions WHERE business_id=$1 AND user_id=$2 AND permission_code='REPORTS_READ'`, f.businessID, f.userID); err != nil {
		t.Fatal(err)
	}
	status, body = f.managementRequest(http.MethodGet, fmt.Sprintf("/outlets/%d/reports/income", f.outletID), nil, token)
	if status != http.StatusForbidden {
		t.Fatalf("staff report permission status=%d body=%s", status, body)
	}
	status, body = f.managementRequest(http.MethodGet, fmt.Sprintf("/outlets/%d/reports/income", f.outletID), nil, "")
	if status != http.StatusUnauthorized {
		t.Fatalf("anonymous reports request status=%d body=%s", status, body)
	}
	adminToken := f.ownerLogin(t)
	status, body = f.managementRequest(http.MethodGet, fmt.Sprintf("/outlets/%d/reports/income?fromDate=2026-09-21&toDate=2026-09-22", f.outletID), nil, adminToken)
	if status != http.StatusOK {
		t.Fatalf("ADMIN should bypass staff report grants within the tenant: status=%d body=%s", status, body)
	}
	var foreignBusinessID, foreignOutletID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO businesses(name) VALUES('Foreign report tenant') RETURNING id`).Scan(&foreignBusinessID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO outlets(business_id,code,name) VALUES($1,'REPORT-FOREIGN','Foreign report outlet') RETURNING id`, foreignBusinessID).Scan(&foreignOutletID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = f.pool.Exec(ctx, `DELETE FROM outlets WHERE business_id=$1`, foreignBusinessID)
		_, _ = f.pool.Exec(ctx, `DELETE FROM businesses WHERE id=$1`, foreignBusinessID)
	}()
	status, body = f.managementRequest(http.MethodGet, fmt.Sprintf("/outlets/%d/reports/income?fromDate=2026-09-21", foreignOutletID), nil, adminToken)
	if status != http.StatusNotFound {
		t.Fatalf("ADMIN must remain inside authenticated business: status=%d body=%s", status, body)
	}
}
