package main

import (
	"bytes"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func notifyEnviron(dsn string) []string {
	return []string{"MONACO_ENV=test", "DATABASE_URL=" + dsn, "NATS_URL=nats://unused"}
}

func notifyTest(t *testing.T, pool *pgxpool.Pool, clk clock.Clock, user string) (int, string, string) {
	t.Helper()
	var stdout, stderr bytes.Buffer
	code := notifyTool(
		notifyEnviron(pool.Config().ConnString()),
		clk,
	)(
		[]string{"test", "--user", user},
		&stdout,
		&stderr,
	)
	return code, stdout.String(), stderr.String()
}

func TestNotifyTest_rejectsMalformedArgumentsWithTheUsage(t *testing.T) {
	t.Parallel()
	environ := notifyEnviron("postgres://127.0.0.1:1/monaco?connect_timeout=1")
	v7, v5 := ids.Real{}.NewV7().String(), uuid.NewSHA1(uuid.NameSpaceOID, []byte("notify-test")).String()
	for _, args := range [][]string{
		{"test"},
		{"test", "--user", "not-a-uuid"},
		{"test", "--user", v5},
		{"test", "--user", strings.ToUpper(v7)},
		{"test", "--user", v7, "extra"},
		{"test", "--cabal", v7},
	} {
		var stdout, stderr bytes.Buffer
		code := run(nil, tools(environ), environ, append([]string{"notify"}, args...), &stdout, &stderr)
		if code != 2 || stderr.String() != notifyUsage+"\n" {
			t.Errorf("notify %v = %d, stderr %q, want 2 and the usage", args, code, stderr.String())
		}
	}
}

func TestNotifyTest_appendsTheRequestAsMonacoctlAndPrintsItsEventID(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user, at := ids.Real{}.NewV7().String(), time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	code, stdout, stderr := notifyTest(t, pool, testkit.NewClock(at), user)

	var (
		id, actor, payloadUser string
		created                time.Time
	)
	if err := pool.QueryRow(t.Context(), `SELECT id::text, actor_type || ':' || actor_id, payload->>'user_id', created_at
		FROM events WHERE type = 'notify.test_requested'`).
		Scan(&id, &actor, &payloadUser, &created); err != nil {
		t.Fatalf("notify test = %d, stderr %q; read the event: %v", code, stderr, err)
	}
	if code != 0 || stdout != id+"\n" || actor != "system:monacoctl" || payloadUser != user || !created.Equal(at) {
		t.Fatalf("notify test = %d, stdout %q, event %s by %s for %s at %v; want 0, the event id, monacoctl, %s, %v",
			code, stdout, id, actor, payloadUser, created, user, at)
	}
}

func TestNotifyTest_reportsAFailedAppend(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `ALTER TABLE events ADD CHECK (type <> 'notify.test_requested')`); err != nil {
		t.Fatal(err)
	}

	code, stdout, stderr := notifyTest(t, pool, clock.Real{}, ids.Real{}.NewV7().String())

	if code != 1 || stdout != "" || !strings.HasPrefix(stderr, "monacoctl: ") {
		t.Fatalf("notify test = %d, stdout %q, stderr %q, want 1, nothing printed and the error", code, stdout, stderr)
	}
}

func TestNotifyTest_reportsAnUnreachableDatabase(t *testing.T) {
	t.Parallel()
	args := []string{"test", "--user", ids.Real{}.NewV7().String()}
	var stdout, stderr bytes.Buffer
	if code := notifyTool(notifyEnviron("postgres://127.0.0.1:1/monaco?connect_timeout=1"), clock.Real{})(
		args, &stdout, &stderr); code != 1 || !strings.Contains(stderr.String(), "monacoctl: db.Open: ") {
		t.Fatalf("notify test with no database = %d, stderr %q, want 1 and db.Open", code, stderr.String())
	}
}
