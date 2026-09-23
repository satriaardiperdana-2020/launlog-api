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

	"github.com/satriaardiperdana-2020/launlog-api/internal/security"
)

func TestManagementTenantPermissionsAndAudit(t *testing.T) {
	f := newAuthFixture(t)
	staffToken := f.login(t).Tokens.AccessToken
	if status, _ := f.managementRequest(http.MethodGet, "/outlets", nil, staffToken); status != http.StatusForbidden {
		t.Fatalf("laundry staff must not manage outlets: status=%d", status)
	}
	ownerToken := f.ownerLogin(t)

	status, response := f.managementRequest(http.MethodPost, "/outlets", map[string]any{"code": "BRANCH-2", "name": "Second Branch"}, ownerToken)
	if status != http.StatusCreated {
		t.Fatalf("create outlet status=%d: %s", status, response)
	}
	var outlet struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(response, &outlet); err != nil || outlet.ID < 1 {
		t.Fatalf("decode created outlet: id=%d err=%v", outlet.ID, err)
	}

	var foreignBusinessID, foreignOutletID int64
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO businesses(name) VALUES('Foreign Management Test') RETURNING id`).Scan(&foreignBusinessID); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(context.Background(), `INSERT INTO outlets(business_id,code,name) VALUES($1,'FOREIGN','Foreign outlet') RETURNING id`, foreignBusinessID).Scan(&foreignOutletID); err != nil {
		t.Fatal(err)
	}
	defer func() {
		_, _ = f.pool.Exec(context.Background(), "DELETE FROM outlets WHERE business_id=$1", foreignBusinessID)
		_, _ = f.pool.Exec(context.Background(), "DELETE FROM businesses WHERE id=$1", foreignBusinessID)
	}()

	email := fmt.Sprintf("staff-%d@example.test", time.Now().UnixNano())
	status, response = f.managementRequest(http.MethodPost, "/staff", map[string]any{
		"email": email, "fullName": "Managed Staff", "password": "strong-password-123", "outletIds": []int64{outlet.ID},
	}, ownerToken)
	if status != http.StatusCreated {
		t.Fatalf("create staff status=%d: %s", status, response)
	}
	var staff struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(response, &staff); err != nil || staff.ID < 1 {
		t.Fatalf("decode created staff: id=%d err=%v", staff.ID, err)
	}

	status, response = f.managementRequest(http.MethodPut, fmt.Sprintf("/staff/%d/outlets", staff.ID), map[string]any{"outletIds": []int64{foreignOutletID}}, ownerToken)
	if status != http.StatusBadRequest {
		t.Fatalf("cross-business assignment must be rejected: status=%d body=%s", status, response)
	}
	var assignmentCount int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM user_outlets WHERE business_id=$1 AND user_id=$2 AND outlet_id=$3", f.businessID, staff.ID, outlet.ID).Scan(&assignmentCount); err != nil || assignmentCount != 1 {
		t.Fatalf("failed cross-business write changed assignments: count=%d err=%v", assignmentCount, err)
	}

	status, response = f.managementRequest(http.MethodPut, fmt.Sprintf("/staff/%d/permissions", staff.ID), map[string]any{"permissions": []string{"ORDERS_READ", "ORDERS_CREATE"}}, ownerToken)
	if status != http.StatusOK {
		t.Fatalf("replace grants status=%d: %s", status, response)
	}
	status, response = f.managementRequest(http.MethodPut, fmt.Sprintf("/staff/%d/permissions", f.userID), map[string]any{"permissions": []string{"AUDIT_READ"}}, ownerToken)
	if status != http.StatusBadRequest {
		t.Fatalf("self grant must be rejected: status=%d body=%s", status, response)
	}
	var grantCount int
	if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM user_permissions WHERE business_id=$1 AND user_id=$2", f.businessID, f.userID).Scan(&grantCount); err != nil || grantCount != 0 {
		t.Fatalf("self-escalation changed grants: count=%d err=%v", grantCount, err)
	}

	var auditCount int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE business_id=$1 AND actor_user_id=$2 AND action IN ('OUTLET_CREATED','STAFF_CREATED','STAFF_PERMISSIONS_REPLACED')`, f.businessID, f.userID).Scan(&auditCount); err != nil || auditCount != 3 {
		t.Fatalf("expected atomic audit entries for management changes: count=%d err=%v", auditCount, err)
	}
	var secretsInAudit int
	if err := f.pool.QueryRow(context.Background(), `SELECT count(*) FROM audit_logs WHERE business_id=$1 AND (COALESCE(new_values::text,'') ILIKE '%password%' OR COALESCE(old_values::text,'') ILIKE '%password%')`, f.businessID).Scan(&secretsInAudit); err != nil || secretsInAudit != 0 {
		t.Fatalf("password material must not be audited: count=%d err=%v", secretsInAudit, err)
	}
}

func TestManagementConcurrentOwnerDeactivationKeepsAnOwner(t *testing.T) {
	f := newAuthFixture(t)
	ownerAToken := f.ownerLogin(t)
	ctx := context.Background()
	ownerBEmail := fmt.Sprintf("admin-%d@example.test", time.Now().UnixNano())
	ownerBHash, err := security.HashPassword("correct-password")
	if err != nil {
		t.Fatal(err)
	}
	var ownerBID int64
	if err := f.pool.QueryRow(ctx, `INSERT INTO users(business_id,email,full_name,password_hash,role) VALUES($1,$2,'Second owner',$3,'ADMIN') RETURNING id`, f.businessID, ownerBEmail, ownerBHash).Scan(&ownerBID); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(ctx, "INSERT INTO user_outlets(business_id,user_id,outlet_id) VALUES($1,$2,$3)", f.businessID, ownerBID, f.outletID); err != nil {
		t.Fatal(err)
	}
	code, body := f.request("/auth/login", map[string]string{"email": ownerBEmail, "password": "correct-password"}, "")
	if code != http.StatusOK {
		t.Fatalf("second owner login status=%d: %s", code, body)
	}
	var login struct {
		Tokens struct {
			AccessToken string `json:"accessToken"`
		} `json:"tokens"`
	}
	if err := json.Unmarshal(body, &login); err != nil || login.Tokens.AccessToken == "" {
		t.Fatalf("decode second owner login: %v", err)
	}

	start := make(chan struct{})
	statuses := make(chan int, 2)
	var wg sync.WaitGroup
	for _, attempt := range []struct {
		token  string
		target int64
	}{{ownerAToken, ownerBID}, {login.Tokens.AccessToken, f.userID}} {
		wg.Add(1)
		go func(token string, target int64) {
			defer wg.Done()
			<-start
			status, _ := f.managementRequest(http.MethodPut, fmt.Sprintf("/staff/%d", target), map[string]any{"fullName": "Owner", "isActive": false}, token)
			statuses <- status
		}(attempt.token, attempt.target)
	}
	close(start)
	wg.Wait()
	close(statuses)
	for status := range statuses {
		if status != http.StatusOK && status != http.StatusUnauthorized && status != http.StatusConflict {
			t.Fatalf("unexpected owner deactivation race response: %d", status)
		}
	}
	var activeOwners int
	if err := f.pool.QueryRow(ctx, "SELECT count(*) FROM users WHERE business_id=$1 AND role='ADMIN' AND is_active", f.businessID).Scan(&activeOwners); err != nil {
		t.Fatal(err)
	}
	if activeOwners < 1 {
		t.Fatal("concurrent owner management removed the last active administrator")
	}
}
