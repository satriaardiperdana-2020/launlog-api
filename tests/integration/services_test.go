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

func TestServicesLifecycleTenantPermissionsAndSnapshots(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	for _, code := range []string{"SERVICES_READ", "SERVICES_WRITE"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO user_permissions(business_id,user_id,permission_code,granted_by) VALUES($1,$2,$3,$2)`, f.businessID, f.userID, code); err != nil {
			t.Fatal(err)
		}
	}
	token := f.login(t).Tokens.AccessToken
	create := map[string]any{"name": "  Wash Fold  ", "description": "  Gentle wash  ", "unit": "KILOGRAM", "unitPriceAmount": int64(12501), "estimatedDurationMinutes": 180}
	status, body := f.managementRequest(http.MethodPost, "/services", create, token)
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	var item struct {
		ID                       int64   `json:"id"`
		Name                     string  `json:"name"`
		Description              *string `json:"description"`
		Unit                     string  `json:"unit"`
		UnitPriceAmount          int64   `json:"unit_price_amount"`
		EstimatedDurationMinutes int32   `json:"estimated_duration_minutes"`
		IsActive                 bool    `json:"is_active"`
	}
	if err := json.Unmarshal(body, &item); err != nil || item.ID < 1 || item.Name != "Wash Fold" || item.Description == nil || *item.Description != "Gentle wash" || item.Unit != "KILOGRAM" || item.UnitPriceAmount != 12501 || !item.IsActive {
		t.Fatalf("created service: %+v err=%v body=%s", item, err, body)
	}
	servicePath := fmt.Sprintf("/services/%d", item.ID)
	for _, tc := range []struct {
		label string
		patch map[string]any
		want  int
	}{
		{"duplicate name and unit", create, http.StatusConflict},
		{"lowercase unit", map[string]any{"name": "Lower", "unit": "kilogram", "unitPriceAmount": 1}, http.StatusBadRequest},
		{"negative price", map[string]any{"name": "Negative", "unit": "PIECE", "unitPriceAmount": -1}, http.StatusBadRequest},
		{"negative duration", map[string]any{"name": "Negative", "unit": "PIECE", "unitPriceAmount": 1, "estimatedDurationMinutes": -1}, http.StatusBadRequest},
		{"missing price", map[string]any{"name": "Missing", "unit": "PIECE"}, http.StatusBadRequest},
		{"body business override", map[string]any{"name": "Override", "unit": "PIECE", "unitPriceAmount": 1, "business_id": 999}, http.StatusBadRequest},
	} {
		code, response := f.managementRequest(http.MethodPost, "/services", tc.patch, token)
		if code != tc.want {
			t.Errorf("%s: got %d want %d body=%s", tc.label, code, tc.want, response)
		}
	}
	status, body = f.managementRequest(http.MethodPost, "/services", map[string]any{"name": "Button", "unit": "PIECE", "unitPriceAmount": 0}, token)
	if status != http.StatusCreated {
		t.Fatalf("zero whole-rupiah price should follow database CHECK: %d %s", status, body)
	}
	status, body = f.managementRequest(http.MethodGet, "/services?q=gentle&unit=KILOGRAM&isActive=true", nil, token)
	if status != http.StatusOK {
		t.Fatalf("filter: %d %s", status, body)
	}
	var page struct {
		Items      []map[string]any `json:"items"`
		Pagination struct {
			TotalItems int64 `json:"totalItems"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(body, &page); err != nil || page.Pagination.TotalItems != 1 || len(page.Items) != 1 {
		t.Fatalf("filtered list: %+v err=%v body=%s", page, err, body)
	}
	status, _ = f.managementRequest(http.MethodGet, "/services?unit=kilogram", nil, token)
	if status != http.StatusBadRequest {
		t.Fatalf("invalid unit filter: %d", status)
	}

	var customerID, orderID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO customers(business_id,name) VALUES($1,'Snapshot Customer') RETURNING id`, f.businessID).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO orders(business_id,outlet_id,customer_id,invoice_number,total_amount,created_by) VALUES($1,$2,$3,'SERVICE-SNAPSHOT-1',12501,$4) RETURNING id`, f.businessID, f.outletID, customerID, f.userID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO order_items(business_id,outlet_id,order_id,service_id,service_name_snapshot,service_unit_snapshot,unit_price_amount_snapshot,quantity,line_total_amount) VALUES($1,$2,$3,$4,'Wash Fold','KILOGRAM',12501,1.000,12501)`, f.businessID, f.outletID, orderID, item.ID); err != nil {
		t.Fatal(err)
	}
	update := map[string]any{"name": "Express Wash", "description": "Fast", "unit": "METER", "unitPriceAmount": int64(19999), "estimatedDurationMinutes": 60, "isActive": false}
	status, body = f.managementRequest(http.MethodPut, servicePath, update, token)
	if status != http.StatusOK {
		t.Fatalf("deactivate and update: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &item); err != nil || item.IsActive || item.Unit != "METER" || item.UnitPriceAmount != 19999 || item.EstimatedDurationMinutes != 60 {
		t.Fatalf("updated service: %+v err=%v", item, err)
	}
	status, body = f.managementRequest(http.MethodGet, "/services?isActive=false&unit=METER", nil, token)
	if status != http.StatusOK || json.Unmarshal(body, &page) != nil || page.Pagination.TotalItems != 1 {
		t.Fatalf("inactive filter: %d %s", status, body)
	}
	update["isActive"] = true
	status, body = f.managementRequest(http.MethodPut, servicePath, update, token)
	if status != http.StatusOK {
		t.Fatalf("reactivate: %d %s", status, body)
	}
	status, body = f.managementRequest(http.MethodDelete, servicePath, nil, token)
	if status != http.StatusNoContent {
		t.Fatalf("soft delete: %d %s", status, body)
	}
	status, _ = f.managementRequest(http.MethodGet, servicePath, nil, token)
	if status != http.StatusNotFound {
		t.Fatalf("deleted service visible: %d", status)
	}
	var name, unit string
	var price int64
	if err := f.pool.QueryRow(ctx, `SELECT service_name_snapshot,service_unit_snapshot,unit_price_amount_snapshot FROM order_items WHERE business_id=$1 AND order_id=$2`, f.businessID, orderID).Scan(&name, &unit, &price); err != nil || name != "Wash Fold" || unit != "KILOGRAM" || price != 12501 {
		t.Fatalf("historical snapshots changed: %q %q %d err=%v", name, unit, price, err)
	}
	var deletedAt *time.Time
	if err := f.pool.QueryRow(ctx, `SELECT deleted_at FROM services WHERE business_id=$1 AND id=$2`, f.businessID, item.ID).Scan(&deletedAt); err != nil || deletedAt == nil {
		t.Fatalf("soft delete did not retain row: %v %v", deletedAt, err)
	}
	var auditCount int64
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE business_id=$1 AND entity_type='service' AND entity_id=$2`, f.businessID, item.ID).Scan(&auditCount); err != nil || auditCount != 4 {
		t.Fatalf("service audits: count=%d err=%v", auditCount, err)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM user_permissions WHERE business_id=$1 AND user_id=$2 AND permission_code='SERVICES_WRITE'`, f.businessID, f.userID); err != nil {
		t.Fatal(err)
	}
	status, _ = f.managementRequest(http.MethodPost, "/services", create, token)
	if status != http.StatusForbidden {
		t.Fatalf("write without grant: %d", status)
	}
	var foreignBusinessID, foreignServiceID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO businesses(name) VALUES($1) RETURNING id`, fmt.Sprintf("Foreign Service %d", time.Now().UnixNano())).Scan(&foreignBusinessID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO services(business_id,name,unit,unit_price_amount) VALUES($1,'Foreign','PIECE',1000) RETURNING id`, foreignBusinessID).Scan(&foreignServiceID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = f.pool.Exec(ctx, `DELETE FROM services WHERE business_id=$1`, foreignBusinessID)
		_, _ = f.pool.Exec(ctx, `DELETE FROM businesses WHERE id=$1`, foreignBusinessID)
	}()
	status, _ = f.managementRequest(http.MethodGet, fmt.Sprintf("/services/%d", foreignServiceID), nil, token)
	if status != http.StatusNotFound {
		t.Fatalf("foreign service detail: %d", status)
	}
	status, _ = f.managementRequest(http.MethodPut, fmt.Sprintf("/services/%d", foreignServiceID), update, token)
	if status != http.StatusForbidden {
		t.Fatalf("write grant removal must block foreign update before lookup: %d", status)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO user_permissions(business_id,user_id,permission_code,granted_by) VALUES($1,$2,'SERVICES_WRITE',$2)`, f.businessID, f.userID); err != nil {
		t.Fatal(err)
	}
	status, _ = f.managementRequest(http.MethodPut, fmt.Sprintf("/services/%d", foreignServiceID), update, token)
	if status != http.StatusNotFound {
		t.Fatalf("foreign service update: %d", status)
	}
	status, _ = f.managementRequest(http.MethodDelete, fmt.Sprintf("/services/%d", foreignServiceID), nil, token)
	if status != http.StatusNotFound {
		t.Fatalf("foreign service delete: %d", status)
	}
}
