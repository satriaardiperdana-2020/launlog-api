//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"sync"
	"testing"
	"time"
)

func TestOrderLifecycleConcurrencyCancellationAndHistory(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	for _, code := range []string{"ORDERS_READ", "ORDERS_UPDATE"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO user_permissions(business_id,user_id,permission_code,granted_by) VALUES($1,$2,$3,$2)`, f.businessID, f.userID, code); err != nil {
			t.Fatal(err)
		}
	}
	var customerID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO customers(business_id,name) VALUES($1,'Lifecycle customer') RETURNING id`, f.businessID).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	createOrder := func(paymentStatus string) int64 {
		t.Helper()
		var id int64
		invoice := fmt.Sprintf("LIFE-%s-%d", paymentStatus, time.Now().UnixNano())
		if err := f.pool.QueryRow(ctx, `INSERT INTO orders(business_id,outlet_id,customer_id,invoice_number,total_amount,payment_status,created_by) VALUES($1,$2,$3,$4,10000,$5,$6) RETURNING id`, f.businessID, f.outletID, customerID, invoice, paymentStatus, f.userID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, `INSERT INTO order_status_history(business_id,outlet_id,order_id,from_status,to_status,changed_by) VALUES($1,$2,$3,NULL,'RECEIVED',$4)`, f.businessID, f.outletID, id, f.userID); err != nil {
			t.Fatal(err)
		}
		return id
	}
	token := f.login(t).Tokens.AccessToken
	orderID := createOrder("UNPAID")
	patchStatus := func(status string) int {
		t.Helper()
		code, _ := f.managementRequest(http.MethodPatch, fmt.Sprintf("/orders/%d/status", orderID), map[string]string{"status": status}, token)
		return code
	}
	if got := patchStatus("READY_FOR_PICKUP"); got != http.StatusConflict {
		t.Fatalf("skip transition status=%d, want 409", got)
	}
	if got := patchStatus("PROCESSING"); got != http.StatusOK {
		t.Fatalf("RECEIVED to PROCESSING status=%d, want 200", got)
	}

	var wg sync.WaitGroup
	statuses := make(chan int, 2)
	for i := 0; i < 2; i++ {
		wg.Add(1)
		go func() { defer wg.Done(); statuses <- patchStatus("READY_FOR_PICKUP") }()
	}
	wg.Wait()
	close(statuses)
	var succeeded, conflicted int
	for status := range statuses {
		if status == http.StatusOK {
			succeeded++
		}
		if status == http.StatusConflict {
			conflicted++
		}
	}
	if succeeded != 1 || conflicted != 1 {
		t.Fatalf("concurrent transition results: successes=%d conflicts=%d", succeeded, conflicted)
	}
	if got := patchStatus("COMPLETED"); got != http.StatusOK {
		t.Fatalf("complete status=%d, want 200", got)
	}
	var completedAt *time.Time
	if err := f.pool.QueryRow(ctx, `SELECT completed_at FROM orders WHERE business_id=$1 AND id=$2`, f.businessID, orderID).Scan(&completedAt); err != nil || completedAt == nil {
		t.Fatalf("completed_at=%v err=%v", completedAt, err)
	}
	if got, _ := f.managementRequest(http.MethodPost, fmt.Sprintf("/orders/%d/cancel", orderID), map[string]string{"reason": "too late"}, token); got != http.StatusConflict {
		t.Fatalf("terminal cancellation status=%d, want 409", got)
	}

	paidOrderID := createOrder("PAID")
	status, body := f.managementRequest(http.MethodPost, fmt.Sprintf("/orders/%d/cancel", paidOrderID), map[string]string{"reason": "customer request"}, token)
	if status != http.StatusConflict {
		t.Fatalf("paid cancellation status=%d want 409: %s", status, body)
	}
	var paidOrder struct {
		Code string `json:"code"`
	}
	_ = json.Unmarshal(body, &paidOrder)
	if paidOrder.Code != "REFUND_POLICY_REQUIRED" {
		t.Fatalf("paid cancellation code=%q body=%s", paidOrder.Code, body)
	}

	cancelID := createOrder("UNPAID")
	status, body = f.managementRequest(http.MethodPost, fmt.Sprintf("/orders/%d/cancel", cancelID), map[string]string{"reason": "customer changed mind"}, token)
	if status != http.StatusOK {
		t.Fatalf("unpaid cancellation status=%d: %s", status, body)
	}
	var gotCancellation struct {
		Status             string     `json:"status"`
		CancelledAt        *time.Time `json:"cancelled_at"`
		CancellationReason *string    `json:"cancellation_reason"`
	}
	if err := json.Unmarshal(body, &gotCancellation); err != nil || gotCancellation.Status != "CANCELLED" || gotCancellation.CancelledAt == nil || gotCancellation.CancellationReason == nil || *gotCancellation.CancellationReason != "customer changed mind" {
		t.Fatalf("cancellation response=%+v err=%v body=%s", gotCancellation, err, body)
	}
	var deletedAt, cancelledAt *time.Time
	var cancelledBy *int64
	var reason *string
	if err := f.pool.QueryRow(ctx, `SELECT deleted_at,cancelled_at,cancelled_by,cancellation_reason FROM orders WHERE business_id=$1 AND id=$2`, f.businessID, cancelID).Scan(&deletedAt, &cancelledAt, &cancelledBy, &reason); err != nil || deletedAt == nil || cancelledAt == nil || cancelledBy == nil || *cancelledBy != f.userID || reason == nil {
		t.Fatalf("cancellation persistence deleted=%v cancelled=%v actor=%v reason=%v err=%v", deletedAt, cancelledAt, cancelledBy, reason, err)
	}
	var historyCount, auditCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM order_status_history WHERE business_id=$1 AND order_id=$2`, f.businessID, cancelID).Scan(&historyCount); err != nil || historyCount != 2 {
		t.Fatalf("history count=%d err=%v", historyCount, err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE business_id=$1 AND entity_type='order' AND entity_id=$2 AND action='ORDER_CANCELLED'`, f.businessID, cancelID).Scan(&auditCount); err != nil || auditCount != 1 {
		t.Fatalf("cancel audit count=%d err=%v", auditCount, err)
	}
	status, body = f.managementRequest(http.MethodGet, fmt.Sprintf("/orders/%d/status-history", cancelID), nil, token)
	if status != http.StatusOK {
		t.Fatalf("cancelled order history status=%d body=%s", status, body)
	}
}
