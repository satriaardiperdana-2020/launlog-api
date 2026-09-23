//go:build integration

package integration_test

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"testing"
	"time"
)

func TestPaymentsIdempotencySettlementCashChangeAndVoid(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	for _, code := range []string{"PAYMENTS_READ", "PAYMENTS_RECORD"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO user_permissions(business_id,user_id,permission_code,granted_by) VALUES($1,$2,$3,$2)`, f.businessID, f.userID, code); err != nil {
			t.Fatal(err)
		}
	}
	var customerID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO customers(business_id,name) VALUES($1,'Payment customer') RETURNING id`, f.businessID).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	createOrder := func(total int64) int64 {
		t.Helper()
		var id int64
		invoice := fmt.Sprintf("PAY-%d", time.Now().UnixNano())
		if err := f.pool.QueryRow(ctx, `INSERT INTO orders(business_id,outlet_id,customer_id,invoice_number,total_amount,created_by) VALUES($1,$2,$3,$4,$5,$6) RETURNING id`, f.businessID, f.outletID, customerID, invoice, total, f.userID).Scan(&id); err != nil {
			t.Fatal(err)
		}
		return id
	}
	token := f.login(t).Tokens.AccessToken
	request := func(method, path, key string, body any) (int, []byte, string) {
		t.Helper()
		var encoded []byte
		if body != nil {
			encoded, _ = json.Marshal(body)
		}
		req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		if key != "" {
			req.Header.Set("Idempotency-Key", key)
		}
		rec := httptest.NewRecorder()
		f.echo.ServeHTTP(rec, req)
		return rec.Code, rec.Body.Bytes(), rec.Header().Get("Idempotency-Replayed")
	}

	orderID := createOrder(10000)
	path := fmt.Sprintf("/orders/%d/payments", orderID)
	foreign := newAuthFixture(t)
	var foreignCustomer, foreignOrder int64
	if err := foreign.pool.QueryRow(ctx, `INSERT INTO customers(business_id,name) VALUES($1,'Foreign payment customer') RETURNING id`, foreign.businessID).Scan(&foreignCustomer); err != nil {
		t.Fatal(err)
	}
	if err := foreign.pool.QueryRow(ctx, `INSERT INTO orders(business_id,outlet_id,customer_id,invoice_number,total_amount,created_by) VALUES($1,$2,$3,$4,5000,$5) RETURNING id`, foreign.businessID, foreign.outletID, foreignCustomer, fmt.Sprintf("FOREIGN-PAY-%d", time.Now().UnixNano()), foreign.userID).Scan(&foreignOrder); err != nil {
		t.Fatal(err)
	}
	if foreignOrder == orderID {
		t.Fatal("test fixtures unexpectedly share an order ID")
	}
	if status, body, _ := request(http.MethodGet, fmt.Sprintf("/orders/%d/payments", foreignOrder), "", nil); status != http.StatusNotFound {
		t.Fatalf("cross-business payment history status=%d body=%s", status, body)
	}
	status, body, replay := request(http.MethodPost, path, "partial-1", map[string]any{"amount": 4000, "method": "BCA_TRANSFER"})
	if status != http.StatusCreated || replay != "" {
		t.Fatalf("first partial status=%d replay=%q body=%s", status, replay, body)
	}
	var first struct {
		Payment struct {
			Amount int64 `json:"amount"`
		} `json:"payment"`
		PaymentStatus string `json:"payment_status"`
		PaidAmount    int64  `json:"paid_amount"`
		Outstanding   int64  `json:"outstanding_amount"`
	}
	if err := json.Unmarshal(body, &first); err != nil || first.Payment.Amount != 4000 || first.PaymentStatus != "PARTIALLY_PAID" || first.PaidAmount != 4000 || first.Outstanding != 6000 {
		t.Fatalf("partial response=%+v err=%v body=%s", first, err, body)
	}
	status, _, replay = request(http.MethodPost, path, "partial-1", map[string]any{"amount": 4000, "method": "BCA_TRANSFER"})
	if status != http.StatusCreated || replay != "true" {
		t.Fatalf("idempotent replay status=%d header=%q", status, replay)
	}
	status, body, _ = request(http.MethodPost, path, "partial-1", map[string]any{"amount": 4500, "method": "BCA_TRANSFER"})
	if status != http.StatusConflict {
		t.Fatalf("idempotency payload conflict status=%d body=%s", status, body)
	}
	var rowCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM payments WHERE business_id=$1 AND order_id=$2`, f.businessID, orderID).Scan(&rowCount); err != nil || rowCount != 1 {
		t.Fatalf("idempotent payment row count=%d err=%v", rowCount, err)
	}
	status, body, _ = request(http.MethodPost, path, "overpay", map[string]any{"amount": 6001, "method": "QRIS"})
	if status != http.StatusConflict {
		t.Fatalf("non-cash overpayment status=%d body=%s", status, body)
	}
	results := make(chan int, 2)
	var concurrent sync.WaitGroup
	for _, key := range []string{"concurrent-a", "concurrent-b"} {
		concurrent.Add(1)
		go func(key string) {
			defer concurrent.Done()
			code, _, _ := request(http.MethodPost, path, key, map[string]any{"amount": 4000, "method": "QRIS"})
			results <- code
		}(key)
	}
	concurrent.Wait()
	close(results)
	var recorded, rejected int
	for code := range results {
		if code == http.StatusCreated {
			recorded++
		}
		if code == http.StatusConflict {
			rejected++
		}
	}
	if recorded != 1 || rejected != 1 {
		t.Fatalf("concurrent payment results: recorded=%d rejected=%d", recorded, rejected)
	}
	status, body, _ = request(http.MethodPost, path, "settle", map[string]any{"amount": 2000, "method": "QRIS"})
	if status != http.StatusCreated {
		t.Fatalf("settlement status=%d body=%s", status, body)
	}
	var settled struct {
		PaymentStatus string `json:"payment_status"`
		PaidAmount    int64  `json:"paid_amount"`
		Outstanding   int64  `json:"outstanding_amount"`
	}
	if err := json.Unmarshal(body, &settled); err != nil || settled.PaymentStatus != "PAID" || settled.PaidAmount != 10000 || settled.Outstanding != 0 {
		t.Fatalf("settlement=%+v err=%v body=%s", settled, err, body)
	}
	status, body, _ = request(http.MethodGet, path, "", nil)
	var summary struct {
		OrderTotal  int64 `json:"order_total"`
		PaidAmount  int64 `json:"paid_amount"`
		Outstanding int64 `json:"outstanding_amount"`
		Items       []struct {
			Amount int64  `json:"amount"`
			Method string `json:"method"`
		} `json:"items"`
	}
	if status != http.StatusOK || json.Unmarshal(body, &summary) != nil || summary.OrderTotal != 10000 || summary.PaidAmount != 10000 || summary.Outstanding != 0 || len(summary.Items) != 3 {
		t.Fatalf("settled ledger status=%d summary=%+v body=%s", status, summary, body)
	}

	cashOrder := createOrder(8000)
	status, body, _ = request(http.MethodPost, fmt.Sprintf("/orders/%d/payments", cashOrder), "cash-over", map[string]any{"amount": 10000, "method": "CASH"})
	var cash struct {
		Payment struct {
			Amount int64 `json:"amount"`
		} `json:"payment"`
		Tendered int64 `json:"tendered_amount"`
		Change   int64 `json:"change_amount"`
	}
	if status != http.StatusCreated || json.Unmarshal(body, &cash) != nil || cash.Payment.Amount != 8000 || cash.Tendered != 10000 || cash.Change != 2000 {
		t.Fatalf("cash change status=%d result=%+v body=%s", status, cash, body)
	}
	var storedCashAmount int64
	if err := f.pool.QueryRow(ctx, `SELECT amount FROM payments WHERE business_id=$1 AND order_id=$2`, f.businessID, cashOrder).Scan(&storedCashAmount); err != nil || storedCashAmount != 8000 {
		t.Fatalf("stored cash revenue=%d err=%v", storedCashAmount, err)
	}
	var cashStatus string
	if err := f.pool.QueryRow(ctx, `SELECT payment_status FROM orders WHERE business_id=$1 AND id=$2`, f.businessID, cashOrder).Scan(&cashStatus); err != nil || cashStatus != "PAID" {
		t.Fatalf("cash order payment status=%q err=%v", cashStatus, err)
	}
	var paymentID int64
	if err := f.pool.QueryRow(ctx, `SELECT id FROM payments WHERE business_id=$1 AND order_id=$2`, f.businessID, cashOrder).Scan(&paymentID); err != nil {
		t.Fatal(err)
	}
	status, body, _ = request(http.MethodPost, fmt.Sprintf("/orders/%d/payments/%d/void", cashOrder, paymentID), "", map[string]string{"reason": "wrong entry"})
	if status != http.StatusOK {
		t.Fatalf("void correction status=%d body=%s", status, body)
	}
	status, body, _ = request(http.MethodPost, fmt.Sprintf("/orders/%d/payments/%d/void", cashOrder, paymentID), "", map[string]string{"reason": "wrong entry"})
	if status != http.StatusOK {
		t.Fatalf("idempotent void retry status=%d body=%s", status, body)
	}
	if err := f.pool.QueryRow(ctx, `SELECT payment_status FROM orders WHERE business_id=$1 AND id=$2`, f.businessID, cashOrder).Scan(&cashStatus); err != nil || cashStatus != "UNPAID" {
		t.Fatalf("voided order payment status=%q err=%v", cashStatus, err)
	}
	var voidAudits int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE business_id=$1 AND entity_type='payment' AND entity_id=$2 AND action='PAYMENT_VOIDED'`, f.businessID, paymentID).Scan(&voidAudits); err != nil || voidAudits != 1 {
		t.Fatalf("void audit count=%d err=%v", voidAudits, err)
	}
}

func TestPaymentAndCancellationSerializeOnOrder(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	for _, code := range []string{"PAYMENTS_RECORD", "ORDERS_UPDATE"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO user_permissions(business_id,user_id,permission_code,granted_by) VALUES($1,$2,$3,$2)`, f.businessID, f.userID, code); err != nil {
			t.Fatal(err)
		}
	}
	var customerID, orderID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO customers(business_id,name) VALUES($1,'Race customer') RETURNING id`, f.businessID).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO orders(business_id,outlet_id,customer_id,invoice_number,total_amount,created_by) VALUES($1,$2,$3,$4,10000,$5) RETURNING id`, f.businessID, f.outletID, customerID, fmt.Sprintf("RACE-%d", time.Now().UnixNano()), f.userID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	token := f.login(t).Tokens.AccessToken
	type result struct {
		status int
		body   []byte
	}
	results := make(chan result, 2)
	var wg sync.WaitGroup
	call := func(method, path string, body any) {
		defer wg.Done()
		encoded, _ := json.Marshal(body)
		req := httptest.NewRequest(method, path, bytes.NewReader(encoded))
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Authorization", "Bearer "+token)
		req.Header.Set("Idempotency-Key", "pay-cancel-race")
		rec := httptest.NewRecorder()
		f.echo.ServeHTTP(rec, req)
		results <- result{rec.Code, rec.Body.Bytes()}
	}
	wg.Add(2)
	go call(http.MethodPost, fmt.Sprintf("/orders/%d/payments", orderID), map[string]any{"amount": 1000, "method": "CASH"})
	go call(http.MethodPost, fmt.Sprintf("/orders/%d/cancel", orderID), map[string]string{"reason": "cancel race"})
	wg.Wait()
	close(results)
	var successes, conflicts int
	for got := range results {
		if got.status == http.StatusCreated || got.status == http.StatusOK {
			successes++
		}
		if got.status == http.StatusConflict {
			conflicts++
		}
		if got.status != http.StatusCreated && got.status != http.StatusOK && got.status != http.StatusConflict {
			t.Errorf("unexpected race response %d: %s", got.status, got.body)
		}
	}
	if successes != 1 || conflicts != 1 {
		t.Fatalf("payment/cancel race successes=%d conflicts=%d", successes, conflicts)
	}
	var paymentStatus, orderStatus string
	var paidCount int
	if err := f.pool.QueryRow(ctx, `SELECT payment_status,status FROM orders WHERE business_id=$1 AND id=$2`, f.businessID, orderID).Scan(&paymentStatus, &orderStatus); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM payments WHERE business_id=$1 AND order_id=$2 AND status='CONFIRMED'`, f.businessID, orderID).Scan(&paidCount); err != nil {
		t.Fatal(err)
	}
	if orderStatus == "CANCELLED" && (paymentStatus != "UNPAID" || paidCount != 0) {
		t.Fatalf("cancelled order has payment state %s count=%d", paymentStatus, paidCount)
	}
	if orderStatus != "CANCELLED" && (paymentStatus != "PARTIALLY_PAID" || paidCount != 1) {
		t.Fatalf("non-cancelled order has payment state %s count=%d", paymentStatus, paidCount)
	}
}
