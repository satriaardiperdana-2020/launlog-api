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

func TestPerfumeLifecycleTenantPermissionsAndAudit(t *testing.T) {
	f := newAuthFixture(t)
	ctx := context.Background()
	for _, code := range []string{"PERFUMES_READ", "PERFUMES_WRITE"} {
		if _, err := f.pool.Exec(ctx, `INSERT INTO user_permissions(business_id,user_id,permission_code,granted_by) VALUES($1,$2,$3,$2)`, f.businessID, f.userID, code); err != nil {
			t.Fatal(err)
		}
	}
	token := f.login(t).Tokens.AccessToken
	create := map[string]any{"name": "Lavender", "description": "Soft floral"}
	status, body := f.managementRequest(http.MethodPost, "/perfumes", create, token)
	if status != http.StatusCreated {
		t.Fatalf("create: %d %s", status, body)
	}
	var item struct {
		ID          int64   `json:"id"`
		Name        string  `json:"name"`
		Description *string `json:"description"`
		IsActive    bool    `json:"is_active"`
	}
	if err := json.Unmarshal(body, &item); err != nil || item.ID <= 0 || item.Name != "Lavender" || item.Description == nil || *item.Description != "Soft floral" || !item.IsActive {
		t.Fatalf("created perfume: %+v err=%v body=%s", item, err, body)
	}

	status, body = f.managementRequest(http.MethodPost, "/perfumes", create, token)
	if status != http.StatusConflict {
		t.Fatalf("duplicate business perfume name: %d %s", status, body)
	}
	status, body = f.managementRequest(http.MethodGet, "/perfumes?q=lavender&page=1&pageSize=10", nil, token)
	if status != http.StatusOK {
		t.Fatalf("search: %d %s", status, body)
	}
	var page struct {
		Items []struct {
			ID int64 `json:"id"`
		} `json:"items"`
		Pagination struct {
			TotalItems int64 `json:"totalItems"`
		} `json:"pagination"`
	}
	if err := json.Unmarshal(body, &page); err != nil || len(page.Items) != 1 || page.Items[0].ID != item.ID || page.Pagination.TotalItems != 1 {
		t.Fatalf("search response: %+v err=%v body=%s", page, err, body)
	}

	path := fmt.Sprintf("/perfumes/%d", item.ID)
	status, body = f.managementRequest(http.MethodPut, path, map[string]any{"name": "Lavender", "description": "Updated", "isActive": false}, token)
	if status != http.StatusOK {
		t.Fatalf("deactivate: %d %s", status, body)
	}
	if err := json.Unmarshal(body, &item); err != nil || item.IsActive || item.Description == nil || *item.Description != "Updated" {
		t.Fatalf("updated perfume: %+v err=%v", item, err)
	}
	status, body = f.managementRequest(http.MethodGet, "/perfumes?isActive=false", nil, token)
	if status != http.StatusOK || json.Unmarshal(body, &page) != nil || page.Pagination.TotalItems != 1 {
		t.Fatalf("inactive filter: %d %s", status, body)
	}

	var deletedAt *time.Time
	if err := f.pool.QueryRow(ctx, `SELECT deleted_at FROM perfumes WHERE business_id=$1 AND id=$2`, f.businessID, item.ID).Scan(&deletedAt); err != nil || deletedAt != nil {
		t.Fatalf("deactivation must not delete perfume: deleted_at=%v err=%v", deletedAt, err)
	}
	var foreignBusinessID, foreignPerfumeID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO businesses(name) VALUES($1) RETURNING id`, fmt.Sprintf("Foreign perfume %d", time.Now().UnixNano())).Scan(&foreignBusinessID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(ctx, `INSERT INTO perfumes(business_id,name) VALUES($1,'Lavender') RETURNING id`, foreignBusinessID).Scan(&foreignPerfumeID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = f.pool.Exec(ctx, `DELETE FROM perfumes WHERE business_id=$1`, foreignBusinessID)
		_, _ = f.pool.Exec(ctx, `DELETE FROM businesses WHERE id=$1`, foreignBusinessID)
	}()
	status, _ = f.managementRequest(http.MethodGet, fmt.Sprintf("/perfumes/%d", foreignPerfumeID), nil, token)
	if status != http.StatusNotFound {
		t.Fatalf("cross-business detail: %d", status)
	}
	if _, err := f.pool.Exec(ctx, `DELETE FROM user_permissions WHERE business_id=$1 AND user_id=$2 AND permission_code='PERFUMES_WRITE'`, f.businessID, f.userID); err != nil {
		t.Fatal(err)
	}
	status, _ = f.managementRequest(http.MethodPut, fmt.Sprintf("/perfumes/%d", foreignPerfumeID), map[string]any{"name": "Other", "isActive": true}, token)
	if status != http.StatusForbidden {
		t.Fatalf("write without grant: %d", status)
	}
	if _, err := f.pool.Exec(ctx, `INSERT INTO user_permissions(business_id,user_id,permission_code,granted_by) VALUES($1,$2,'PERFUMES_WRITE',$2)`, f.businessID, f.userID); err != nil {
		t.Fatal(err)
	}
	status, _ = f.managementRequest(http.MethodPut, fmt.Sprintf("/perfumes/%d", foreignPerfumeID), map[string]any{"name": "Other", "isActive": true}, token)
	if status != http.StatusNotFound {
		t.Fatalf("cross-business update: %d", status)
	}

	status, body = f.managementRequest(http.MethodDelete, path, nil, token)
	if status != http.StatusNoContent {
		t.Fatalf("soft delete: %d %s", status, body)
	}
	status, _ = f.managementRequest(http.MethodGet, path, nil, token)
	if status != http.StatusNotFound {
		t.Fatalf("deleted perfume remains visible: %d", status)
	}
	var auditCount int64
	if err := f.pool.QueryRow(ctx, `SELECT count(*) FROM audit_logs WHERE business_id=$1 AND entity_type='perfume' AND entity_id=$2`, f.businessID, item.ID).Scan(&auditCount); err != nil || auditCount != 3 {
		t.Fatalf("perfume audit events: count=%d err=%v", auditCount, err)
	}
}
