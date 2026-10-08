//go:build integration

package integration_test

import (
	"context"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"

	"github.com/jackc/pgx/v5/pgxpool"
)

func TestPlatformBootstrapRestrictedAndConcurrent(t *testing.T) {
	bootstrapURL := os.Getenv("TEST_BOOTSTRAP_DATABASE_URL")
	adminURL := os.Getenv("TEST_ADMIN_DATABASE_URL")
	if bootstrapURL == "" || adminURL == "" {
		t.Skip("TEST_BOOTSTRAP_DATABASE_URL and TEST_ADMIN_DATABASE_URL are required")
	}
	ctx := context.Background()
	admin, err := pgxpool.New(ctx, adminURL)
	if err != nil {
		t.Fatal(err)
	}
	var existing int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM platform_admins").Scan(&existing); err != nil || existing != 0 {
		t.Fatalf("bootstrap test requires empty platform table: count=%d err=%v", existing, err)
	}
	t.Cleanup(func() {
		_, _ = admin.Exec(ctx, "ALTER TABLE platform_audit_logs DISABLE TRIGGER platform_audit_logs_immutable")
		_, _ = admin.Exec(ctx, "DELETE FROM platform_audit_logs WHERE action='PLATFORM_ADMIN_BOOTSTRAPPED'")
		_, _ = admin.Exec(ctx, "ALTER TABLE platform_audit_logs ENABLE TRIGGER platform_audit_logs_immutable")
		_, _ = admin.Exec(ctx, "DELETE FROM platform_admins WHERE id=1")
		admin.Close()
	})
	var canInsertAdmin, canUpdateAudit bool
	bootstrapPool, err := pgxpool.New(ctx, bootstrapURL)
	if err != nil {
		t.Fatal(err)
	}
	defer bootstrapPool.Close()
	if err := bootstrapPool.QueryRow(ctx, `SELECT has_table_privilege(current_user,'public.platform_admins','INSERT'),
		has_table_privilege(current_user,'public.platform_audit_logs','UPDATE')`).Scan(&canInsertAdmin, &canUpdateAudit); err != nil || !canInsertAdmin || canUpdateAudit {
		t.Fatalf("bootstrap role privileges unexpected: insert=%t audit_update=%t err=%v", canInsertAdmin, canUpdateAudit, err)
	}
	binary := filepath.Join(t.TempDir(), "platform-bootstrap")
	build := exec.Command(filepath.Join(runtime.GOROOT(), "bin", "go"), "build", "-o", binary, "./cmd/platform-bootstrap")
	build.Dir = filepath.Join("..", "..")
	if output, err := build.CombinedOutput(); err != nil {
		t.Fatalf("build platform bootstrap: %v: %s", err, output)
	}
	invoke := func() (string, error) {
		read, write, err := os.Pipe()
		if err != nil {
			return "", err
		}
		defer read.Close()
		if _, err := io.WriteString(write, "platform-secret-password\n"); err != nil {
			write.Close()
			return "", err
		}
		write.Close()
		cmd := exec.Command(binary, "--email", "platform@example.test")
		cmd.Env = append(os.Environ(), "APP_ENV=integration", "PLATFORM_BOOTSTRAP_DATABASE_URL="+bootstrapURL)
		cmd.ExtraFiles = []*os.File{read}
		output, err := cmd.CombinedOutput()
		return string(output), err
	}
	var results [2]struct {
		output string
		err    error
	}
	var wg sync.WaitGroup
	for i := range results {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			results[i].output, results[i].err = invoke()
		}(i)
	}
	wg.Wait()
	successes := 0
	for _, result := range results {
		if strings.Contains(result.output, "platform-secret-password") {
			t.Fatal("bootstrap exposed password")
		}
		if result.err == nil {
			successes++
		}
	}
	if successes != 1 {
		t.Fatalf("concurrent bootstrap successes=%d, want one; results=%+v", successes, results)
	}
	var admins, audits int
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM platform_admins WHERE is_active").Scan(&admins); err != nil {
		t.Fatal(err)
	}
	if err := admin.QueryRow(ctx, "SELECT count(*) FROM platform_audit_logs WHERE action='PLATFORM_ADMIN_BOOTSTRAPPED'").Scan(&audits); err != nil {
		t.Fatal(err)
	}
	if admins != 1 || audits != 1 {
		t.Fatalf("bootstrap committed admins=%d audits=%d, want one each", admins, audits)
	}
}
