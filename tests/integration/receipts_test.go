//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"testing"
)

func TestReceiptAuthorizationSnapshotsAndTemplateManagement(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	var customerID, serviceID, orderID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO customers (business_id,name,phone) VALUES ($1,'Receipt-time customer','+628111111111') RETURNING id`, f.businessID).Scan(&customerID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO services (business_id,name,unit,unit_price_amount) VALUES ($1,'Shirt','PIECE',12500) RETURNING id`, f.businessID).Scan(&serviceID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO orders (business_id,outlet_id,customer_id,invoice_number,total_amount,created_by) VALUES ($1,$2,$3,'AUTH-RECEIPT-1',12500,$4) RETURNING id`, f.businessID, f.outletID, customerID, f.userID).Scan(&orderID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO order_items (business_id,outlet_id,order_id,service_id,service_name_snapshot,service_unit_snapshot,unit_price_amount_snapshot,quantity,line_total_amount) VALUES ($1,$2,$3,$4,'Shirt at order time','PIECE',12500,1,12500)`, f.businessID, f.outletID, orderID, serviceID); err != nil {
		t.Fatal(err)
	}
	var qrID, snapshotName string
	if err := f.pool.QueryRow(ctx, `SELECT receipt_qr_id,customer_name_snapshot FROM orders WHERE business_id=$1 AND id=$2`, f.businessID, orderID).Scan(&qrID, &snapshotName); err != nil {
		t.Fatal(err)
	}
	if snapshotName != "Receipt-time customer" {
		t.Fatalf("receipt customer snapshot=%q", snapshotName)
	}
	if _, err := f.pool.Exec(ctx, `UPDATE customers SET name='Changed later',phone='+628199999999' WHERE business_id=$1 AND id=$2`, f.businessID, customerID); err != nil {
		t.Fatal(err)
	}
	token := f.login(t).Tokens.AccessToken
	qrPath := fmt.Sprintf("/outlets/%d/receipts/qr/%s", f.outletID, qrID)
	if code, _ := f.managementRequest(http.MethodGet, qrPath, nil, ""); code != http.StatusUnauthorized {
		t.Fatalf("anonymous QR lookup status=%d", code)
	}
	if code, _ := f.managementRequest(http.MethodGet, qrPath, nil, token); code != http.StatusForbidden {
		t.Fatalf("ungranted QR lookup status=%d", code)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO user_permissions (business_id,user_id,permission_code,granted_by) VALUES ($1,$2,'ORDERS_READ',$2),($1,$2,'RECEIPTS_MANAGE',$2)`, f.businessID, f.userID); err != nil {
		t.Fatal(err)
	}
	token = f.login(t).Tokens.AccessToken
	code, body := f.managementRequest(http.MethodGet, qrPath, nil, token)
	if code != http.StatusOK {
		t.Fatalf("authorized QR lookup status=%d: %s", code, body)
	}
	var receipt struct {
		Order struct {
			Customer struct {
				Name  string  `json:"name"`
				Phone *string `json:"phone"`
			} `json:"customer"`
		} `json:"order"`
		Items []struct {
			ServiceName string `json:"serviceName"`
		} `json:"items"`
		WhatsAppShare struct {
			Phone   *string `json:"phone"`
			Message *string `json:"message"`
		} `json:"whatsappShare"`
	}
	if err := json.Unmarshal(body, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.Order.Customer.Name != "Receipt-time customer" || receipt.Order.Customer.Phone == nil || *receipt.Order.Customer.Phone != "+628111111111" {
		t.Fatalf("receipt customer snapshot not preserved: %+v", receipt.Order.Customer)
	}
	if len(receipt.Items) != 1 || receipt.Items[0].ServiceName != "Shirt at order time" {
		t.Fatalf("receipt line item snapshot not returned: %+v", receipt.Items)
	}
	if receipt.WhatsAppShare.Phone == nil || *receipt.WhatsAppShare.Phone != "+628111111111" {
		t.Fatalf("WhatsApp phone is not receipt-time value: %+v", receipt.WhatsAppShare)
	}

	templatePath := fmt.Sprintf("/outlets/%d/receipt-templates", f.outletID)
	input := map[string]any{"name": "Front desk", "paperWidthMm": 80, "whatsappMessageTemplate": "Hi {{customer_name}}: {{order_total}}"}
	code, body = f.managementRequest(http.MethodPost, templatePath, input, token)
	if code != http.StatusCreated {
		t.Fatalf("create template status=%d: %s", code, body)
	}
	var created struct {
		ID        int64 `json:"id"`
		IsDefault bool  `json:"isDefault"`
	}
	if err := json.Unmarshal(body, &created); err != nil {
		t.Fatal(err)
	}
	if !created.IsDefault {
		t.Fatal("first outlet template should be default")
	}
	input["name"] = "Unsupported"
	input["whatsappMessageTemplate"] = "{{customer_email}}"
	if code, body = f.managementRequest(http.MethodPost, templatePath, input, token); code != http.StatusBadRequest {
		t.Fatalf("unsupported placeholder status=%d body=%s", code, body)
	}
	if code, body = f.managementRequest(http.MethodGet, fmt.Sprintf("/orders/%d/receipt", orderID), nil, token); code != http.StatusOK {
		t.Fatalf("order receipt status=%d: %s", code, body)
	}
	if err := json.Unmarshal(body, &receipt); err != nil {
		t.Fatal(err)
	}
	if receipt.WhatsAppShare.Message == nil || *receipt.WhatsAppShare.Message != "Hi Receipt-time customer: Rp 12.500" {
		t.Fatalf("share text did not render from receipt snapshots: %+v", receipt.WhatsAppShare)
	}
	input = map[string]any{"name": "Back counter", "paperWidthMm": 58, "isDefault": true}
	code, body = f.managementRequest(http.MethodPost, templatePath, input, token)
	if code != http.StatusCreated {
		t.Fatalf("create replacement default status=%d: %s", code, body)
	}
	var replacement struct {
		ID        int64 `json:"id"`
		IsDefault bool  `json:"isDefault"`
	}
	if err := json.Unmarshal(body, &replacement); err != nil {
		t.Fatal(err)
	}
	var defaultCount int
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM receipt_templates WHERE business_id=$1 AND outlet_id=$2 AND is_default AND deleted_at IS NULL`, f.businessID, f.outletID).Scan(&defaultCount); err != nil {
		t.Fatal(err)
	}
	if defaultCount != 1 || !replacement.IsDefault {
		t.Fatalf("default invariant broken: count=%d replacement=%+v", defaultCount, replacement)
	}
	deletePath := fmt.Sprintf("%s/%d", templatePath, replacement.ID)
	if code, body = f.managementRequest(http.MethodDelete, deletePath, nil, token); code != http.StatusOK {
		t.Fatalf("delete template status=%d: %s", code, body)
	}
	var firstStillDefault bool
	if err := f.pool.QueryRow(ctx, `SELECT is_default FROM receipt_templates WHERE id=$1`, created.ID).Scan(&firstStillDefault); err != nil {
		t.Fatal(err)
	}
	if !firstStillDefault {
		t.Fatal("deleting the default should promote the next active template")
	}
}
