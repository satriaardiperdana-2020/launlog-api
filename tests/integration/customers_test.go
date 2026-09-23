//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"strings"
	"testing"
	"time"
)

func TestCustomerLifecycleHistoryAndTenantPermissions(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	for _, code := range []string{"CUSTOMERS_READ", "CUSTOMERS_WRITE", "ORDERS_READ"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO user_permissions(business_id,user_id,permission_code,granted_by) VALUES($1,$2,$3,$2)`, f.businessID, f.userID, code); err != nil {
			t.Fatal(err)
		}
	}
	token := f.login(t).Tokens.AccessToken

	status, response := f.managementRequest(http.MethodPost, "/customers", map[string]any{"name": "  Emma Laundry  ", "phone": "+628123000111", "address": "Jakarta"}, token)
	if status != http.StatusCreated {
		t.Fatalf("create customer status=%d: %s", status, response)
	}
	var customer struct {
		ID      int64   `json:"id"`
		Name    string  `json:"name"`
		Phone   *string `json:"phone"`
		Address *string `json:"address"`
	}
	if err := json.Unmarshal(response, &customer); err != nil || customer.ID < 1 || customer.Name != "Emma Laundry" || customer.Phone == nil || *customer.Phone != "+628123000111" {
		t.Fatalf("decode customer response: customer=%+v err=%v", customer, err)
	}
	status, response = f.managementRequest(http.MethodPost, "/customers", map[string]any{"name": "Another Emma", "phone": "+628123000111"}, token)
	if status != http.StatusCreated {
		t.Fatalf("duplicate phone values must be accepted: status=%d body=%s", status, response)
	}
	status, response = f.managementRequest(http.MethodPost, "/customers", map[string]any{"phone": "+628123000222"}, token)
	if status != http.StatusBadRequest {
		t.Fatalf("missing required customer name must fail: status=%d body=%s", status, response)
	}

	searchPath := "/customers?q=" + url.QueryEscape("emma") + "&page=1&pageSize=10"
	status, response = f.managementRequest(http.MethodGet, searchPath, nil, token)
	if status != http.StatusOK {
		t.Fatalf("customer search status=%d: %s", status, response)
	}
	var list struct {
		Items      []map[string]any `json:"items"`
		Pagination struct {
			TotalItems int64 `json:"totalItems"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(response, &list); err != nil || list.Pagination.TotalItems != 2 {
		t.Fatalf("customer search should find both same-phone customers: total=%d err=%v body=%s", list.Pagination.TotalItems, err, response)
	}

	status, response = f.managementRequest(http.MethodPut, fmt.Sprintf("/customers/%d", customer.ID), map[string]any{"name": "Emma Updated", "address": "Updated address"}, token)
	if status != http.StatusOK {
		t.Fatalf("update customer status=%d: %s", status, response)
	}
	if err := json.Unmarshal(response, &customer); err != nil || customer.Name != "Emma Updated" || customer.Phone != nil || customer.Address == nil || *customer.Address != "Updated address" {
		t.Fatalf("omitted optional fields should clear on full update: customer=%+v err=%v", customer, err)
	}

	var otherOutletID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO outlets(business_id,code,name) VALUES($1,'HISTORY-2','History outlet 2') RETURNING id`, f.businessID).Scan(&otherOutletID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO orders(business_id,outlet_id,customer_id,invoice_number,total_amount,created_by) VALUES($1,$2,$3,'CUST-HIST-001',8000,$4)`, f.businessID, f.outletID, customer.ID, f.userID); err != nil {
		t.Fatal(err)
	}
	var cancelledOrderID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO orders(business_id,outlet_id,customer_id,invoice_number,total_amount,created_by) VALUES($1,$2,$3,'CUST-HIST-002',12000,$4) RETURNING id`, f.businessID, f.outletID, customer.ID, f.userID).Scan(&cancelledOrderID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE orders SET status='CANCELLED',cancelled_at=now(),cancelled_by=$2,cancellation_reason='customer changed mind',deleted_at=now() WHERE business_id=$1 AND id=$3`, f.businessID, f.userID, cancelledOrderID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO orders(business_id,outlet_id,customer_id,invoice_number,total_amount,created_by) VALUES($1,$2,$3,'CUST-HIST-003',2500,$4)`, f.businessID, otherOutletID, customer.ID, f.userID); err != nil {
		t.Fatal(err)
	}

	status, response = f.managementRequest(http.MethodGet, fmt.Sprintf("/customers/%d/orders?page=1&pageSize=1", customer.ID), nil, token)
	if status != http.StatusOK {
		t.Fatalf("customer history status=%d: %s", status, response)
	}
	var history struct {
		Items []struct {
			ID            int64  `json:"id"`
			Status        string `json:"status"`
			TotalAmount   int64  `json:"total_amount"`
			InvoiceNumber string `json:"invoice_number"`
		} `json:"items"`
		Pagination struct {
			TotalItems int64 `json:"totalItems"`
			TotalPages int64 `json:"totalPages"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(response, &history); err != nil || len(history.Items) != 1 || history.Pagination.TotalItems != 2 || history.Pagination.TotalPages != 2 {
		t.Fatalf("customer history should be paginated and outlet-scoped: history=%+v err=%v body=%s", history, err, response)
	}
	if history.Items[0].ID != cancelledOrderID || history.Items[0].Status != "CANCELLED" || history.Items[0].TotalAmount != 12000 {
		t.Fatalf("history should use actual order data and include cancelled orders: %+v", history.Items[0])
	}

	var foreignBusinessID, foreignCustomerID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO businesses(name) VALUES($1) RETURNING id`, fmt.Sprintf("Customer Tenant %d", time.Now().UnixNano())).Scan(&foreignBusinessID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO customers(business_id,name) VALUES($1,'Foreign Customer') RETURNING id`, foreignBusinessID).Scan(&foreignCustomerID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = f.pool.Exec(ctx, "DELETE FROM customers WHERE business_id=$1", foreignBusinessID)
		_, _ = f.pool.Exec(ctx, "DELETE FROM businesses WHERE id=$1", foreignBusinessID)
	}()
	status, _ = f.managementRequest(http.MethodGet, fmt.Sprintf("/customers/%d", foreignCustomerID), nil, token)
	if status != http.StatusNotFound {
		t.Fatalf("cross-business customer detail must be hidden: status=%d", status)
	}

	status, response = f.managementRequest(http.MethodDelete, fmt.Sprintf("/customers/%d", customer.ID), nil, token)
	if status != http.StatusNoContent {
		t.Fatalf("customer deactivation status=%d: %s", status, response)
	}
	status, _ = f.managementRequest(http.MethodGet, fmt.Sprintf("/customers/%d", customer.ID), nil, token)
	if status != http.StatusNotFound {
		t.Fatalf("deactivated customer should not be returned by active detail endpoint: status=%d", status)
	}
	status, response = f.managementRequest(http.MethodGet, fmt.Sprintf("/customers/%d/orders?page=1&pageSize=10", customer.ID), nil, token)
	if status != http.StatusOK {
		t.Fatalf("historical order references should remain accessible after deactivation: status=%d body=%s", status, response)
	}
	var retainedOrderCount int64
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM orders WHERE business_id=$1 AND customer_id=$2", f.businessID, customer.ID).Scan(&retainedOrderCount); err != nil || retainedOrderCount != 3 {
		t.Fatalf("customer deactivation must preserve all order foreign-key references: count=%d err=%v", retainedOrderCount, err)
	}
	var auditCount int64
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE business_id=$1 AND actor_user_id=$2 AND entity_type='customer' AND entity_id=$3 AND action IN ('CUSTOMER_CREATED','CUSTOMER_UPDATED','CUSTOMER_DEACTIVATED')`, f.businessID, f.userID, customer.ID).Scan(&auditCount); err != nil || auditCount != 3 {
		t.Fatalf("customer create/update/deactivation should be audited transactionally: count=%d err=%v", auditCount, err)
	}
	var auditText string
	if err := f.pool.QueryRow(ctx, `SELECT string_agg(coalesce(old_values::text,'') || coalesce(new_values::text,''),' ') FROM audit_logs WHERE business_id=$1 AND entity_type='customer' AND entity_id=$2`, f.businessID, customer.ID).Scan(&auditText); err != nil {
		t.Fatal(err)
	}
	for _, privateValue := range []string{"Emma Laundry", "+628123000111", "Jakarta", "Updated address"} {
		if strings.Contains(auditText, privateValue) {
			t.Fatalf("customer audit leaked personal data %q: %s", privateValue, auditText)
		}
	}
	if !strings.Contains(auditText, "REDACTED") {
		t.Fatalf("customer audit should explicitly mark redacted personal data: %s", auditText)
	}

	if _, err := f.pool.Exec(ctx, "DELETE FROM user_permissions WHERE business_id=$1 AND user_id=$2 AND permission_code='ORDERS_READ'", f.businessID, f.userID); err != nil {
		t.Fatal(err)
	}
	status, _ = f.managementRequest(http.MethodGet, fmt.Sprintf("/customers/%d/orders", customer.ID), nil, token)
	if status != http.StatusForbidden {
		t.Fatalf("customer history must require ORDERS_READ as well as CUSTOMERS_READ: status=%d", status)
	}
}
