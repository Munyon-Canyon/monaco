package governance_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric/noop"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestModule_isNamedGovernanceAndRunsTheExpiryPoller(t *testing.T) {
	t.Parallel()
	m := governance.New(module.Deps{})
	var routes httpx.Routes
	m.Routes(&routes)
	pollers := m.Pollers()
	if m.Name() != "governance" || routes.GovernanceRoutes == nil {
		t.Fatalf("module = %s, routes %+v", m.Name(), routes)
	}
	var handlers []string
	for _, c := range m.Consumers() {
		for _, h := range c.Handlers {
			handlers = append(handlers, c.Durable+" "+h.Name+" "+string(h.Type()))
		}
	}
	want := []string{
		"governance governance.trade_outcome.confirmed trade.confirmed",
		"governance governance.trade_outcome.blocked trade.blocked",
	}
	if !slices.Equal(handlers, want) {
		t.Fatalf("consumers = %q, want %q", handlers, want)
	}
	if len(pollers) != 1 || pollers[0].Name() != "governance.proposal_expiry" ||
		pollers[0].Interval() != 30*time.Second {
		t.Fatalf("pollers = %v, want governance.proposal_expiry every 30s", pollers)
	}
}

type expiryWorker struct {
	mu    sync.Mutex
	lines []string
}

func (w *expiryWorker) Write(p []byte) (int, error) {
	var line struct {
		Msg    string `json:"msg"`
		Poller string `json:"poller"`
	}
	if err := json.Unmarshal(p, &line); err != nil {
		return 0, err
	}
	if line.Poller == "governance.proposal_expiry" && strings.HasPrefix(line.Msg, "poller.") {
		w.mu.Lock()
		w.lines = append(w.lines, line.Msg)
		w.mu.Unlock()
	}
	return len(p), nil
}

func (w *expiryWorker) seen() []string {
	w.mu.Lock()
	defer w.mu.Unlock()
	return append([]string(nil), w.lines...)
}

func startExpiryWorker(t *testing.T, pool *pgxpool.Pool, clk *testkit.Clock) *expiryWorker {
	t.Helper()
	ids := testkit.NewIDs(uint64(clk.Now().UnixNano()))
	pollers := governance.New(module.Deps{Clock: clk, IDs: ids, Pool: pool, UoW: db.New(pool, ids, clk)}).Pollers()
	runner, err := poller.NewRunner(pool, clk, noop.NewMeterProvider().Meter("t"))
	if err != nil {
		t.Fatal(err)
	}
	w := &expiryWorker{}
	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	ctx = observability.WithLogger(ctx, slog.New(slog.NewJSONHandler(w, &slog.HandlerOptions{Level: slog.LevelDebug})))
	var run errgroup.Group
	run.Go(func() error { return runner.Run(ctx, pollers...) })
	t.Cleanup(func() {
		cancel()
		if err := run.Wait(); err != nil {
			t.Errorf("Run returned %v at cleanup", err)
		}
	})
	return w
}

func waitForLines(t *testing.T, w *expiryWorker, n int) []string {
	t.Helper()
	testkit.Eventually(t, func() bool { return len(w.seen()) >= n }, 30*time.Second)
	return w.seen()
}

func TestExpiryPoller_twoWorkersOnOneDatabaseExpireEachProposalOnce(t *testing.T) {
	t.Parallel()
	d := newVoteDB(t)
	soon := d.expiring(t, d.clk.Now().Add(app.ExpiryInterval))
	a := startExpiryWorker(t, d.pool, d.clk)
	if got := waitForLines(t, a, 1); got[0] != "poller.tick" {
		t.Fatalf("first worker logged %v, want a tick", got)
	}
	b := startExpiryWorker(t, d.pool, d.clk)
	if got := waitForLines(t, b, 1); got[0] != "poller.tick.skipped_locked" {
		t.Fatalf("second worker logged %v, want a skip while the first holds the lock", got)
	}
	d.clk.Advance(app.ExpiryInterval)
	if got := waitForLines(t, a, 2); got[1] != "poller.tick" {
		t.Fatalf("first worker logged %v, want a second tick", got)
	}
	if got := waitForLines(t, b, 2); got[1] != "poller.tick.skipped_locked" {
		t.Fatalf("second worker logged %v, want a second skip", got)
	}
	if d.status(t, soon) != "expired" || len(d.payloads(t, soon, events.TypeProposalExpired)) != 1 {
		t.Fatalf("status %s, want expired with exactly one proposal.expired", d.status(t, soon))
	}
}
