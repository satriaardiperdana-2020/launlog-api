//go:build integration

package integration_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"testing"
)

// These fixtures seed existing tenants, not the unimplemented ISSUE-018
// business/outlet/ADMIN provisioning workflow. Requests exercise the real API.
type issue017Tenant struct {
	f            *authFixture
	token, email string
	customerID   int64
}

func issue017Tenants(t *testing.T) [2]issue017Tenant {
	t.Helper()
	for _, key := range []string{"TEST_DATABASE_URL", "TEST_ADMIN_DATABASE_URL"} {
		if os.Getenv(key) == "" {
			t.Fatalf("%s is required; refusing to skip tenant isolation tests", key)
		}
	}
	if os.Getenv("TEST_EXPECT_LEAST_PRIVILEGE") != "true" {
		t.Fatal("TEST_EXPECT_LEAST_PRIVILEGE=true is required")
	}
	var tenants [2]issue017Tenant
	for i, name := range []string{"Golden Laundry 19", "Laundry Matahari"} {
		f := newAuthFixture(t)
		ctx := context.Background()
		if _, err := f.pool.Exec(ctx, "UPDATE businesses SET name=$1 WHERE id=$2", name, f.businessID); err != nil {
			t.Fatal(err)
		}
		if _, err := f.pool.Exec(ctx, "UPDATE users SET full_name=$1 WHERE id=$2", fmt.Sprintf("Admin %c", 'A'+i), f.userID); err != nil {
			t.Fatal(err)
		}
		tenants[i] = issue017Tenant{f: f, token: f.ownerLogin(t)}
		if err := f.pool.QueryRow(ctx, "SELECT email FROM users WHERE id=$1", f.userID).Scan(&tenants[i].email); err != nil {
			t.Fatal(err)
		}
		body := issue017Request(t, f, tenants[i].token, http.MethodPost, "/customers", map[string]any{"name": name + " customer"}, http.StatusCreated)
		tenants[i].customerID = issue017ID(t, body)
	}
	return tenants
}

func issue017Request(t *testing.T, f *authFixture, token, method, path string, input any, want int) []byte {
	t.Helper()
	status, body := f.managementRequest(method, path, input, token)
	if status != want {
		t.Fatalf("%s %s: status=%d want=%d body=%s", method, path, status, want, body)
	}
	return body
}

func issue017ID(t *testing.T, body []byte) int64 {
	t.Helper()
	var record struct {
		ID int64 `json:"id"`
	}
	if err := json.Unmarshal(body, &record); err != nil || record.ID < 1 {
		t.Fatalf("invalid record: %s err=%v", body, err)
	}
	return record.ID
}

func TestIssue017TwoTenantAdminIsolation(t *testing.T) {
	tenants := issue017Tenants(t)
	// Both credentials go through the SAME router and database connection.
	api := tenants[0].f
	for i, own := range tenants {
		foreign := tenants[1-i]
		t.Run(fmt.Sprintf("Admin_%c", 'A'+i), func(t *testing.T) {
			for _, record := range []struct {
				route        string
				own, foreign int64
				update       map[string]any
			}{
				{"outlets", own.f.outletID, foreign.f.outletID, map[string]any{"code": "AUTH", "name": "Changed outlet", "isActive": true}},
				{"staff", own.f.userID, foreign.f.userID, map[string]any{"fullName": "Changed Admin", "isActive": true}},
				{"customers", own.customerID, foreign.customerID, map[string]any{"name": "Changed customer"}},
			} {
				t.Run(record.route, func(t *testing.T) {
					issue017Request(t, api, own.token, http.MethodGet, fmt.Sprintf("/%s/%d", record.route, record.own), nil, http.StatusOK)
					before := issue017Request(t, api, foreign.token, http.MethodGet, fmt.Sprintf("/%s/%d", record.route, record.foreign), nil, http.StatusOK)
					issue017Request(t, api, own.token, http.MethodGet, fmt.Sprintf("/%s/%d", record.route, record.foreign), nil, http.StatusNotFound)
					issue017Request(t, api, own.token, http.MethodPut, fmt.Sprintf("/%s/%d", record.route, record.foreign), record.update, http.StatusNotFound)
					after := issue017Request(t, api, foreign.token, http.MethodGet, fmt.Sprintf("/%s/%d", record.route, record.foreign), nil, http.StatusOK)
					if string(before) != string(after) {
						t.Fatalf("foreign %s changed after rejected write", record.route)
					}
					body := issue017Request(t, api, own.token, http.MethodGet, "/"+record.route, nil, http.StatusOK)
					var page struct {
						Items []struct {
							ID int64 `json:"id"`
						} `json:"items"`
						Pagination struct {
							TotalItems int `json:"totalItems"`
						} `json:"pagination"`
					}
					if err := json.Unmarshal(body, &page); err != nil {
						t.Fatal(err)
					}
					if len(page.Items) != 1 || page.Items[0].ID != record.own || page.Pagination.TotalItems != 1 {
						t.Fatalf("list leaked or omitted tenant data: %s", body)
					}
					issue017Request(t, api, own.token, http.MethodPut, fmt.Sprintf("/%s/%d", record.route, record.own), record.update, http.StatusOK)
				})
			}
			before := issue017Counts(t, own.f)
			issue017Request(t, api, own.token, http.MethodPost, "/staff", map[string]any{"email": fmt.Sprintf("foreign-outlet-%d@example.test", own.f.businessID), "fullName": "Rejected staff", "password": "strong-password-123", "outletIds": []int64{own.f.outletID, foreign.f.outletID}}, http.StatusBadRequest)
			issue017Request(t, api, own.token, http.MethodPut, fmt.Sprintf("/staff/%d/outlets", own.f.userID), map[string]any{"outletIds": []int64{foreign.f.outletID}}, http.StatusBadRequest)
			if after := issue017Counts(t, own.f); before != after {
				t.Fatalf("foreign outlet attempt changed counts: before=%v after=%v", before, after)
			}
			var assigned int64
			if err := own.f.pool.QueryRow(context.Background(), "SELECT outlet_id FROM user_outlets WHERE business_id=$1 AND user_id=$2", own.f.businessID, own.f.userID).Scan(&assigned); err != nil || assigned != own.f.outletID {
				t.Fatalf("assignment changed: %d err=%v", assigned, err)
			}
		})
	}
}

// Include assignments and audit records to catch partially committed writes.
func issue017Counts(t *testing.T, f *authFixture) [4]int {
	t.Helper()
	var counts [4]int
	for i, table := range []string{"outlets", "users", "user_outlets", "audit_logs"} {
		if err := f.pool.QueryRow(context.Background(), "SELECT count(*) FROM "+table+" WHERE business_id=$1", f.businessID).Scan(&counts[i]); err != nil {
			t.Fatal(err)
		}
	}
	return counts
}

func TestIssue017DuplicateEmailAndServerControlledStaffRole(t *testing.T) {
	tenants := issue017Tenants(t)
	api := tenants[0].f
	for i, own := range tenants {
		t.Run(fmt.Sprintf("Admin_%c", 'A'+i), func(t *testing.T) {
			input := func(email string) map[string]any {
				return map[string]any{"email": email, "fullName": "New staff", "password": "strong-password-123", "outletIds": []int64{own.f.outletID}}
			}
			before := issue017Counts(t, own.f)
			for _, email := range []string{own.email, tenants[1-i].email, " " + strings.ToUpper(tenants[1-i].email) + " "} {
				issue017Request(t, api, own.token, http.MethodPost, "/staff", input(email), http.StatusConflict)
			}
			email := fmt.Sprintf("role-test-%d@example.test", own.f.businessID)
			for _, role := range []string{"ADMIN", "PLATFORM_ADMIN"} {
				body := input(email)
				body["role"] = role
				issue017Request(t, api, own.token, http.MethodPost, "/staff", body, http.StatusBadRequest)
			}
			if after := issue017Counts(t, own.f); before != after {
				t.Fatalf("rejected staff creation changed counts: %v -> %v", before, after)
			}
			body := issue017Request(t, api, own.token, http.MethodPost, "/staff", input(email), http.StatusCreated)
			id := issue017ID(t, body)
			var response struct {
				Role string `json:"role"`
			}
			if err := json.Unmarshal(body, &response); err != nil || response.Role != "LAUNDRY_STAFF" {
				t.Fatalf("server role response: %s err=%v", body, err)
			}
			before = issue017Counts(t, own.f)
			for _, role := range []string{"ADMIN", "PLATFORM_ADMIN"} {
				issue017Request(t, api, own.token, http.MethodPut, fmt.Sprintf("/staff/%d", id), map[string]any{"fullName": "Escalated", "isActive": true, "role": role}, http.StatusBadRequest)
			}
			var storedRole, name string
			if err := own.f.pool.QueryRow(context.Background(), "SELECT role,full_name FROM users WHERE id=$1 AND business_id=$2", id, own.f.businessID).Scan(&storedRole, &name); err != nil || storedRole != "LAUNDRY_STAFF" || name != "New staff" {
				t.Fatalf("role escalation persisted: role=%s name=%s err=%v", storedRole, name, err)
			}
			if after := issue017Counts(t, own.f); before != after {
				t.Fatalf("role override added audit or records: %v -> %v", before, after)
			}
		})
	}
}

func TestIssue017ExistingManagementMutationRollback(t *testing.T) {
	tenants := issue017Tenants(t)
	f := tenants[0].f
	token := tenants[0].token
	// Fault injection is confined to this disposable database and this tenant.
	// This tests outlet/staff transactions, not combined tenant provisioning.
	name := fmt.Sprintf("issue017_fail_%d", f.businessID)
	ctx := context.Background()
	function := fmt.Sprintf(`CREATE FUNCTION %s() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN IF NEW.business_id = %d THEN RAISE EXCEPTION 'integration injected failure'; END IF; RETURN NEW; END $$`, name, f.businessID)
	if _, err := f.adminPool.Exec(ctx, function); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if _, err := f.adminPool.Exec(ctx, "DROP FUNCTION "+name+"() CASCADE"); err != nil {
			t.Errorf("cleanup fault injection: %v", err)
		}
	})
	for _, tc := range []struct {
		label, table, path string
		input              map[string]any
	}{
		{"outlet audit failure", "audit_logs", "/outlets", map[string]any{"code": "ROLLBACK", "name": "Must roll back"}},
		{"staff assignment failure", "user_outlets", "/staff", map[string]any{"email": fmt.Sprintf("rollback-%d@example.test", f.businessID), "fullName": "Must roll back", "password": "strong-password-123", "outletIds": []int64{f.outletID}}},
		{"staff audit failure", "audit_logs", "/staff", map[string]any{"email": fmt.Sprintf("rollback-audit-%d@example.test", f.businessID), "fullName": "Must roll back", "password": "strong-password-123", "outletIds": []int64{f.outletID}}},
	} {
		t.Run(tc.label, func(t *testing.T) {
			before := issue017Counts(t, f)
			if _, err := f.adminPool.Exec(ctx, fmt.Sprintf("CREATE TRIGGER %s BEFORE INSERT ON %s FOR EACH ROW EXECUTE FUNCTION %s()", name, tc.table, name)); err != nil {
				t.Fatal(err)
			}
			defer func() {
				if _, err := f.adminPool.Exec(ctx, "DROP TRIGGER "+name+" ON "+tc.table); err != nil {
					t.Errorf("remove fault trigger: %v", err)
				}
			}()
			issue017Request(t, f, token, http.MethodPost, tc.path, tc.input, http.StatusInternalServerError)
			if after := issue017Counts(t, f); before != after {
				t.Fatalf("partial mutation committed after failure: %v -> %v", before, after)
			}
		})
	}
	// A request succeeds after fault removal, proving the payload was valid.
	issue017Request(t, f, token, http.MethodPost, "/outlets", map[string]any{"code": "ROLLBACK", "name": "Recovered outlet"}, http.StatusCreated)
	issue017Request(t, f, token, http.MethodPost, "/staff", map[string]any{"email": fmt.Sprintf("rollback-%d@example.test", f.businessID), "fullName": "Recovered staff", "password": "strong-password-123", "outletIds": []int64{f.outletID}}, http.StatusCreated)
}
