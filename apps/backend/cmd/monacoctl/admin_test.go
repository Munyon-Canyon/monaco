package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
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

func TestAdminServiceToken_createStoresOnlyTheHashAndRevokeEndsIt(t *testing.T) {
	t.Parallel()
	admin, _, pool := adminFixture(t)
	var stdout, stderr bytes.Buffer
	if code := admin(
		[]string{"token", "create", "--name", "grafana"},
		&stdout,
		&stderr,
	); code != 0 ||
		stderr.Len() != 0 {
		t.Fatalf("create = %d, stderr %q", code, stderr.String())
	}
	token := strings.TrimSpace(stdout.String())
	if !domain.IsServiceToken(token) || stdout.String() != token+"\n" {
		t.Fatalf("stdout %q is not one mst_ token", stdout.String())
	}
	assertStoredToken(t, pool, token)
	stdout.Reset()
	if code := admin([]string{"token", "revoke", "--name", "grafana"}, &stdout, &stderr); code != 0 ||
		stdout.String() != "grafana\n" {
		t.Fatalf("revoke = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	var live int
	if err := pool.QueryRow(t.Context(),
		`SELECT count(*) FROM admin_service_tokens WHERE revoked_at IS NULL`).Scan(&live); err != nil || live != 0 {
		t.Fatalf("live tokens = %d, err %v", live, err)
	}
}

func TestAdminServiceToken_refusesDuplicateLiveNamesAndUnknownRevokes(t *testing.T) {
	t.Parallel()
	admin, _, _ := adminFixture(t)
	var stdout, stderr bytes.Buffer
	if code := admin([]string{"token", "create", "--name", "grafana"}, &stdout, &stderr); code != 0 {
		t.Fatalf("first create = %d, stderr %q", code, stderr.String())
	}
	stdout.Reset()
	if code := admin([]string{"token", "create", "--name", "grafana"}, &stdout, &stderr); code != 1 ||
		stdout.Len() != 0 || !strings.Contains(stderr.String(), "a live token already has the name grafana") {
		t.Fatalf("duplicate = %d, stdout %q, stderr %q", code, stdout.String(), stderr.String())
	}
	stderr.Reset()
	if code := admin([]string{"token", "revoke", "--name", "missing"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "no rows") {
		t.Fatalf("revoke missing = %d, stderr %q", code, stderr.String())
	}
	if code := admin([]string{"token", "revoke", "--name", "grafana"}, &stdout, &stderr); code != 0 {
		t.Fatalf("revoke = %d", code)
	}
	stdout.Reset()
	if code := admin([]string{"token", "create", "--name", "grafana"}, &stdout, &stderr); code != 0 {
		t.Fatalf("recreate after revoke = %d, stderr %q", code, stderr.String())
	}
}

func TestAdminServiceToken_rejectsBadArgumentsAndUnavailableDatabase(t *testing.T) {
	t.Parallel()
	admin, _, pool := adminFixture(t)
	for _, args := range [][]string{
		{"token"},
		{"token", "create"},
		{"token", "rotate", "--name", "grafana"},
		{"token", "create", "--name", "Bad Name"},
		{"token", "revoke", "--name", "grafana", "extra"},
		{"token", "create", "--bogus"},
	} {
		var stdout, stderr bytes.Buffer
		if code := admin(
			args,
			&stdout,
			&stderr,
		); code != 2 || stderr.String() != adminNameUsage+"\n" ||
			stdout.Len() != 0 {
			t.Fatalf("admin %v = %d, stderr %q", args, code, stderr.String())
		}
	}
	unavailable := adminTool([]string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=1", "NATS_URL=nats://unused",
	}, testkit.NewClock(time.Now()))
	var stdout, stderr bytes.Buffer
	if code := unavailable([]string{"token", "create", "--name", "grafana"}, &stdout, &stderr); code != 1 ||
		!strings.Contains(stderr.String(), "db_unavailable") || stdout.Len() != 0 {
		t.Fatalf("unavailable = %d, stderr %q", code, stderr.String())
	}
	if _, err := pool.Exec(t.Context(), `DROP TABLE admin_service_tokens`); err != nil {
		t.Fatal(err)
	}
	stderr.Reset()
	if code := admin(
		[]string{"token", "create", "--name", "grafana"},
		&stdout,
		&stderr,
	); code != 1 ||
		stdout.Len() != 0 {
		t.Fatalf("write error = %d, stdout %q", code, stdout.String())
	}
	if code := admin([]string{"token", "revoke", "--name", "grafana"}, &stdout, &stderr); code != 1 {
		t.Fatalf("revoke write error = %d", code)
	}
}

func assertStoredToken(t *testing.T, pool *pgxpool.Pool, token string) {
	t.Helper()
	var name, createdBy string
	var stored []byte
	var createdAt time.Time
	if err := pool.QueryRow(t.Context(),
		`SELECT name, token_hash, created_at, created_by FROM admin_service_tokens WHERE revoked_at IS NULL`).
		Scan(&name, &stored, &createdAt, &createdBy); err != nil {
		t.Fatal(err)
	}
	if name != "grafana" || !bytes.Equal(stored, domain.HashServiceToken(token)) ||
		!createdAt.Equal(time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)) || !strings.HasPrefix(createdBy, "monacoctl") {
		t.Fatalf("row = %q, %x, %v, %q", name, stored, createdAt, createdBy)
	}
	var leaks int
	if err := pool.QueryRow(
		t.Context(),
		`SELECT count(*) FROM admin_service_tokens t WHERE t::text LIKE '%' || $1 || '%'`,
		token,
	).Scan(&leaks); err != nil ||
		leaks != 0 {
		t.Fatalf("raw token appears in %d stored rows, err %v", leaks, err)
	}
}

func TestAdminServiceToken_recordsTheOperatorAndRefusesWhenRandomnessFails(t *testing.T) {
	t.Parallel()
	_, _, pool := adminFixture(t)
	admin := adminTool([]string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=nats://unused", "USER=ops",
	}, testkit.NewClock(time.Now()))
	var stdout, stderr bytes.Buffer
	if code := admin([]string{"token", "create", "--name", "grafana"}, &stdout, &stderr); code != 0 {
		t.Fatalf("create = %d, stderr %q", code, stderr.String())
	}
	var createdBy string
	if err := pool.QueryRow(t.Context(), `SELECT created_by FROM admin_service_tokens`).Scan(&createdBy); err != nil ||
		createdBy != "monacoctl:ops" {
		t.Fatalf("created_by = %q, err %v", createdBy, err)
	}
	issued, err := createServiceToken(t.Context(), sqlc.New(pool), testkit.NewClock(time.Now()),
		bytes.NewReader(nil), "other", "ops")
	if err == nil || issued != "" {
		t.Fatalf("empty randomness issued %q, err %v", issued, err)
	}
}
