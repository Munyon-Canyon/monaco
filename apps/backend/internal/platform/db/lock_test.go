package db_test

import (
	"context"
	"errors"
	"fmt"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const terminateLockHolders = `SELECT count(*) FILTER (WHERE pg_terminate_backend(pid, $1)) FROM (
	SELECT DISTINCT pid FROM pg_locks
	WHERE locktype = 'advisory' AND database = (SELECT oid FROM pg_database WHERE datname = current_database())
) holders`

const lockSessions = `SELECT count(DISTINCT pid) FROM pg_locks
WHERE locktype = 'advisory' AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`

const terminateWait = 5 * time.Second

func newLocks(t *testing.T, pool *pgxpool.Pool, keys ...string) *db.Locks {
	t.Helper()
	l := db.NewLocks(pool)
	t.Cleanup(func() {
		for _, key := range keys {
			if err := l.Release(t.Context(), key); err != nil {
				t.Logf("cleanup release %s: %v", key, err)
			}
		}
	})
	return l
}

func hold(t *testing.T, l *db.Locks, key string) bool {
	t.Helper()
	held, err := l.Hold(t.Context(), key)
	if err != nil {
		t.Fatal(err)
	}
	return held
}

func release(t *testing.T, l *db.Locks, key string) {
	t.Helper()
	if err := l.Release(t.Context(), key); err != nil {
		t.Fatal(err)
	}
}

func TestLock_oneSessionHoldsTheKeyUntilItReleases(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a := newLocks(t, pool, "poller:a", "poller:b")
	b := newLocks(t, pool, "poller:a", "poller:c")
	if first, again := hold(t, a, "poller:a"), hold(t, a, "poller:a"); !first || !again {
		t.Fatalf("holder Hold = %v then %v, want it to take the free key and keep it", first, again)
	}
	if !hold(t, a, "poller:b") {
		t.Fatal("holder could not take a second key")
	}
	if hold(t, b, "poller:a") {
		t.Fatal("second session took a key the first still holds")
	}
	if !hold(t, b, "poller:c") {
		t.Fatal("a different key was blocked")
	}
	if got := pool.Stat().AcquiredConns(); got != 2 {
		t.Fatalf("acquired conns = %d, want one per holder however many keys it holds", got)
	}
	release(t, a, "poller:a")
	release(t, a, "poller:a")
	if !hold(t, b, "poller:a") {
		t.Fatal("key was not free after Release")
	}
	if !hold(t, a, "poller:b") {
		t.Fatal("releasing one key dropped another the holder still holds")
	}
	release(t, a, "poller:b")
	if got := pool.Stat().AcquiredConns(); got != 1 {
		t.Fatalf("acquired conns = %d, want a holder with no keys to give its connection back", got)
	}
}

func TestLock_holdsEveryKeyOnOneSession(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	keys := make([]string, 8)
	for i := range keys {
		keys[i] = fmt.Sprintf("poller:%d", i)
	}
	a := newLocks(t, pool, keys...)
	for _, key := range keys {
		if !hold(t, a, key) {
			t.Fatalf("did not take the free key %s", key)
		}
	}
	var sessions int
	if err := pool.QueryRow(t.Context(), lockSessions).Scan(&sessions); err != nil || sessions != 1 {
		t.Fatalf("sessions holding advisory locks = %d (%v), want 1 for %d keys", sessions, err, len(keys))
	}
}

func TestLock_holderWhoseSessionDiedTakesEveryKeyBackBeforeAnotherSessionCan(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a := newLocks(t, pool, "poller:a", "poller:b")
	b := newLocks(t, pool, "poller:a", "poller:b")
	if !hold(t, a, "poller:a") || !hold(t, a, "poller:b") {
		t.Fatal("did not take the free keys")
	}
	terminateHolders(t, pool)
	for _, key := range []string{"poller:a", "poller:b"} {
		logs := &testkit.Logs{}
		held, err := a.Hold(loggedCtx(t, logs), key)
		if !held || err != nil {
			t.Fatalf("Hold %s after the session died = %v, %v; want the free key taken again", key, held, err)
		}
		assertLostLine(t, logs, key, "true")
		if hold(t, b, key) {
			t.Fatalf("another session took %s after the holder retook it", key)
		}
	}
	if !hold(t, a, "poller:a") {
		t.Fatal("retaken key was lost on the next Hold")
	}
}

func TestLock_holderWhoseSessionDiedLosesTheKeyToAnotherSession(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a := newLocks(t, pool, "poller:a")
	b := newLocks(t, pool, "poller:a")
	if !hold(t, a, "poller:a") {
		t.Fatal("did not take the free key")
	}
	terminateHolders(t, pool)
	if !hold(t, b, "poller:a") {
		t.Fatal("key stayed taken after its session died")
	}
	logs := &testkit.Logs{}
	if held, err := a.Hold(loggedCtx(t, logs), "poller:a"); held || err != nil {
		t.Fatalf("old holder Hold = %v, %v; want the key left to the other session", held, err)
	}
	assertLostLine(t, logs, "poller:a", "false")
	terminateHolders(t, pool)
	if err := b.Release(t.Context(), "poller:a"); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Release after the session died = %v, want db_unavailable", err)
	}
}

func TestLock_holderWhoseSessionDiedReportsWhyItCouldNotRetake(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a := newLocks(t, pool, "poller:a")
	if !hold(t, a, "poller:a") {
		t.Fatal("did not take the free key")
	}
	terminateHolders(t, pool)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	held, err := a.Hold(ctx, "poller:a")
	if held || errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Hold with a dead session and a cancelled context = %v, %v; want false and db_unavailable", held, err)
	}
}

func TestLock_cancelledHoldLeavesTheOtherKeysHeld(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a := newLocks(t, pool, "poller:a")
	b := newLocks(t, pool, "poller:a")
	if !hold(t, a, "poller:a") {
		t.Fatal("did not take the free key")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if held, err := a.Hold(ctx, "poller:b"); held || errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Hold with a cancelled context = %v, %v; want false and db_unavailable", held, err)
	}
	if hold(t, b, "poller:a") {
		t.Fatal("a cancelled Hold for one key gave up another key")
	}
}

func loggedCtx(t *testing.T, logs *testkit.Logs) context.Context {
	t.Helper()
	return observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
}

func assertLostLine(t *testing.T, logs *testkit.Logs, key, held string) {
	t.Helper()
	line := string(logs.Bytes())
	for _, want := range []string{`"level":"WARN"`, `"msg":"db.lock.lost"`, `"lock":"` + key + `"`, `"held":` + held, `"err":`} {
		if !strings.Contains(line, want) {
			t.Fatalf("log = %q, want one db.lock.lost Warn line containing %s", line, want)
		}
	}
	if strings.Count(line, "\n") != 1 {
		t.Fatalf("log = %q, want exactly one line", line)
	}
}

func terminateHolders(t *testing.T, pool *pgxpool.Pool) {
	t.Helper()
	var gone int
	err := pool.QueryRow(t.Context(), terminateLockHolders, terminateWait.Milliseconds()).Scan(&gone)
	if err != nil || gone != 1 {
		t.Fatalf("%d lock sessions exited within %s of pg_terminate_backend (%v), want 1", gone, terminateWait, err)
	}
}

func TestLock_releaseFreesTheKeyEvenWithACancelledContext(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a := newLocks(t, pool, "poller:a")
	b := newLocks(t, pool, "poller:a")
	if !hold(t, a, "poller:a") {
		t.Fatal("did not take the free key")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := a.Release(ctx, "poller:a"); err != nil {
		t.Fatal(err)
	}
	if !hold(t, b, "poller:a") {
		t.Fatal("key was still taken right after Release returned")
	}
}

func TestLock_reportsCodedErrorsWithoutHoldingAConnection(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	held, err := newLocks(t, pool).Hold(t.Context(), "poller:\xff")
	if held || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Hold with a key Postgres rejects = %v, %v; want false and internal", held, err)
	}
	if got := pool.Stat().AcquiredConns(); got != 0 {
		t.Fatalf("acquired conns = %d after a failed Hold, want 0", got)
	}
	closed, err := pgxpool.NewWithConfig(t.Context(), pool.Config())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	held, err = newLocks(t, closed).Hold(t.Context(), "poller:a")
	var coded *errs.Error
	if held || !errors.As(err, &coded) {
		t.Fatalf("Hold on a closed pool = %v, %v; want false and a coded error", held, err)
	}
}
