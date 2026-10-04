package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func adminFixture(t *testing.T) (tool, string, *pgxpool.Pool) {
	t.Helper()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{Handle: "devadmin"})
	return adminTool(
		[]string{"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused"},
		testkit.NewClock(time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)),
	), user.ID.String(), pool
}

func TestAdminTool_grantsAndRevokesWithSystemEvents(t *testing.T) {
	t.Parallel()
	admin, id, _ := adminFixture(t)
	var stdout, stderr bytes.Buffer
	if code := admin([]string{"grant", "--handle", "devadmin", "--role", "operator"}, &stdout, &stderr); code != 0 ||
		stdout.String() != id+"\n" || stderr.Len() != 0 {
		t.Fatalf("grant = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	stdout.Reset()
	if code := admin([]string{"revoke", "--handle", "devadmin"}, &stdout, &stderr); code != 0 ||
		stdout.String() != id+"\n" {
		t.Fatalf("revoke = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	if code := admin([]string{"revoke", "--handle", "devadmin"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "no rows") {
		t.Fatalf("second revoke = %d, stderr %q", code, stderr.String())
	}
}

func TestAdminTool_rejectsBadArgumentsAndUnavailableDatabase(t *testing.T) {
	t.Parallel()
	admin, _, _ := adminFixture(t)
	for _, args := range [][]string{{"grant"}, {"grant", "--handle", "devadmin", "--role", "unknown"}, {"revoke"}} {
		var stdout, stderr bytes.Buffer
		if code := admin(args, &stdout, &stderr); code != 2 || stderr.String() != adminUsage+"\n" {
			t.Fatalf("admin %v = %d, stderr %q", args, code, stderr.String())
		}
	}
	var unknownOut, unknownErr bytes.Buffer
	if code := admin([]string{"unknown"}, &unknownOut, &unknownErr); code != 2 ||
		!strings.Contains(unknownErr.String(), "unknown command") {
		t.Fatalf("unknown = %d, stderr %q", code, unknownErr.String())
	}
	admin = adminTool([]string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1", "NATS_URL=nats://unused",
	}, testkit.NewClock(time.Now()))
	var stdout, stderr bytes.Buffer
	if code := admin([]string{"grant", "--handle", "devadmin", "--role", "viewer"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "db_unavailable") {
		t.Fatalf("unavailable grant = %d, stderr %q", code, stderr.String())
	}
	if toolAdmin(toolEnv{}) == nil || config.EnvTest != "test" {
		t.Fatal("admin tool is unavailable")
	}
}

func TestAdminTool_reportsMissingUsersAndWriteErrors(t *testing.T) {
	t.Parallel()
	admin, _, pool := adminFixture(t)
	var stdout, stderr bytes.Buffer
	if code := admin([]string{"grant", "--handle", "missing", "--role", "viewer"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "admin user by handle") {
		t.Fatalf("missing user = %d, stderr %q", code, stderr.String())
	}
	stderr.Reset()
	if code := admin([]string{"grant", "--handle", "devadmin", "--role", "viewer"}, &stdout, &stderr); code != 0 {
		t.Fatalf("seed grant = %d, stderr %q", code, stderr.String())
	}
	if _, err := pool.Exec(t.Context(), `DROP TABLE admins`); err != nil {
		t.Fatal(err)
	}
	stdout.Reset()
	stderr.Reset()
	if code := admin([]string{"revoke", "--handle", "devadmin"}, &stdout, &stderr); code != 1 {
		t.Fatalf("write revoke = %d, stderr %q", code, stderr.String())
	}
	stderr.Reset()
	if code := admin([]string{"grant", "--handle", "devadmin", "--role", "viewer"}, &stdout, &stderr); code != 1 {
		t.Fatalf("write grant = %d, stderr %q", code, stderr.String())
	}
}
