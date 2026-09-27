package db_test

import (
	"bytes"
	"context"
	"errors"
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

const terminateLockHolders = `SELECT count(*) FILTER (WHERE pg_terminate_backend(pid, $1)) FROM pg_locks
WHERE locktype = 'advisory' AND database = (SELECT oid FROM pg_database WHERE datname = current_database())`

const terminateWait = 5 * time.Second

func newLock(t *testing.T, pool *pgxpool.Pool, key string) *db.Lock {
	t.Helper()
	l := db.NewLock(pool, key)
	t.Cleanup(func() {
		if err := l.Release(t.Context()); err != nil {
			t.Logf("cleanup release: %v", err)
		}
	})
	return l
}

func hold(t *testing.T, l *db.Lock) bool {
	t.Helper()
	held, err := l.Hold(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return held
}

func TestLock_oneSessionHoldsTheKeyUntilItReleases(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a, b := newLock(t, pool, "poller:a"), newLock(t, pool, "poller:a")
	other := newLock(t, pool, "poller:b")
	if first, again := hold(t, a), hold(t, a); !first || !again {
		t.Fatalf("holder Hold = %v then %v, want it to take the free key and keep it", first, again)
	}
	if hold(t, b) {
		t.Fatal("second session took a key the first still holds")
	}
	if !hold(t, other) {
		t.Fatal("a different key was blocked")
	}
	if err := a.Release(t.Context()); err != nil {
		t.Fatal(err)
	}
	if err := a.Release(t.Context()); err != nil {
		t.Fatalf("second Release = %v, want a no-op", err)
	}
	if !hold(t, b) {
		t.Fatal("key was not free after Release")
	}
	if got := pool.Stat().AcquiredConns(); got != 2 {
		t.Fatalf("acquired conns = %d, want only the two holders to keep a connection", got)
	}
	for _, l := range []*db.Lock{b, other} {
		if err := l.Release(t.Context()); err != nil {
			t.Fatal(err)
		}
	}
}

func TestLock_holderWhoseSessionDiedTakesTheKeyBackWhenItIsFree(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a := newLock(t, pool, "poller:a")
	if !hold(t, a) {
		t.Fatal("did not take the free key")
	}
	terminateHolders(t, pool)
	logs := &bytes.Buffer{}
	held, err := a.Hold(loggedCtx(t, logs))
	if !held || err != nil {
		t.Fatalf("Hold after the session died = %v, %v; want the free key taken again", held, err)
	}
	assertLostLine(t, logs, "true")
	if !hold(t, a) {
		t.Fatal("retaken key was lost on the next Hold")
	}
}

func TestLock_holderWhoseSessionDiedLosesTheKeyToAnotherSession(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a, b := newLock(t, pool, "poller:a"), newLock(t, pool, "poller:a")
	if !hold(t, a) {
		t.Fatal("did not take the free key")
	}
	terminateHolders(t, pool)
	if !hold(t, b) {
		t.Fatal("key stayed taken after its session died")
	}
	logs := &bytes.Buffer{}
	if held, err := a.Hold(loggedCtx(t, logs)); held || err != nil {
		t.Fatalf("old holder Hold = %v, %v; want the key left to the other session", held, err)
	}
	assertLostLine(t, logs, "false")
	terminateHolders(t, pool)
	if err := b.Release(t.Context()); errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Release after the session died = %v, want db_unavailable", err)
	}
}

func TestLock_holderWhoseSessionDiedReportsWhyItCouldNotRetake(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	a := newLock(t, pool, "poller:a")
	if !hold(t, a) {
		t.Fatal("did not take the free key")
	}
	terminateHolders(t, pool)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	held, err := a.Hold(ctx)
	if held || errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("Hold with a dead session and a cancelled context = %v, %v; want false and db_unavailable", held, err)
	}
}

func loggedCtx(t *testing.T, logs *bytes.Buffer) context.Context {
	t.Helper()
	return observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
}

func assertLostLine(t *testing.T, logs *bytes.Buffer, held string) {
	t.Helper()
	line := logs.String()
	for _, want := range []string{`"level":"WARN"`, `"msg":"db.lock.lost"`, `"lock":"poller:a"`, `"held":` + held, `"err":`} {
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
	a, b := newLock(t, pool, "poller:a"), newLock(t, pool, "poller:a")
	if !hold(t, a) {
		t.Fatal("did not take the free key")
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if err := a.Release(ctx); err != nil {
		t.Fatal(err)
	}
	if !hold(t, b) {
		t.Fatal("key was still taken right after Release returned")
	}
	if err := b.Release(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func TestLock_reportsCodedErrorsWithoutHoldingAConnection(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	held, err := newLock(t, pool, "poller:\xff").Hold(t.Context())
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
	held, err = newLock(t, closed, "poller:a").Hold(t.Context())
	var coded *errs.Error
	if held || !errors.As(err, &coded) {
		t.Fatalf("Hold on a closed pool = %v, %v; want false and a coded error", held, err)
	}
}
