package identity_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type nudgeWorker struct {
	mu    sync.Mutex
	lines []string
}

func (w *nudgeWorker) Write(p []byte) (int, error) {
	var line struct {
		Msg    string `json:"msg"`
		Poller string `json:"poller"`
	}
	if err := json.Unmarshal(p, &line); err != nil {
		return 0, err
	}
	if line.Poller == "identity.nudges" && strings.HasPrefix(line.Msg, "poller.") {
		w.mu.Lock()
		w.lines = append(w.lines, line.Msg)
		w.mu.Unlock()
	}
	return len(p), nil
}

func (w *nudgeWorker) seen(t *testing.T, n int) []string {
	t.Helper()
	testkit.Eventually(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.lines) >= n
	}, 30*time.Second)
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.lines...)
}

func startNudgeWorker(t *testing.T, r nudgeRig) *nudgeWorker {
	t.Helper()
	runner, err := poller.NewRunner(r.pool, r.clock, noop.NewMeterProvider().Meter("t"))
	if err != nil {
		t.Fatal(err)
	}
	w := &nudgeWorker{}
	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	ctx = observability.WithLogger(ctx, slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})))
	var run errgroup.Group
	run.Go(func() error {
		return runner.Run(ctx, app.NewEmitNudges(r.uow, r.pool, r.clock, time.Hour))
	})
	t.Cleanup(func() {
		cancel()
		if err := run.Wait(); err != nil {
			t.Errorf("Run returned %v at cleanup", err)
		}
	})
	return w
}

func TestEmitNudges_twoWorkersOnOneDatabaseNudgeEachUserOnce(t *testing.T) {
	t.Parallel()
	r := newNudgeRig(t)
	user := seedNudgeRow(t, r.pool, r.clock.Now(), nudgeRow{state: "AWAITING_PHONE", changedAgo: 48 * time.Hour})
	a := startNudgeWorker(t, r)
	if got := a.seen(t, 1); got[0] != "poller.tick" {
		t.Fatalf("first worker logged %v, want a tick", got)
	}
	b := startNudgeWorker(t, r)
	if got := b.seen(t, 1); got[0] != "poller.tick.skipped_locked" {
		t.Fatalf("second worker logged %v, want a skip while the first holds the lock", got)
	}
	r.clock.Advance(time.Hour)
	if got := a.seen(t, 2); got[1] != "poller.tick" {
		t.Fatalf("first worker logged %v, want a second tick", got)
	}
	if got := b.seen(t, 2); got[1] != "poller.tick.skipped_locked" {
		t.Fatalf("second worker logged %v, want a second skip", got)
	}
	if got := nudgesFor(t, r.pool, user); len(got) != 1 {
		t.Fatalf("nudges = %+v, want exactly one", got)
	}
}
