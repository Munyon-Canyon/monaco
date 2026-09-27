package poller_test

import (
	"context"
	"log/slog"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestRunner_onlyTheLockHolderTicksAndTheOtherTakesOverWhenItStops(t *testing.T) {
	t.Parallel()
	pool, clk := testkit.DB(t), testkit.NewClock(epoch())
	a, b := newHarness(t, pool, clk), newHarness(t, pool, clk)
	pa, pb := &fakePoller{name: "test.contended", tick: quiet}, &fakePoller{name: "test.contended", tick: quiet}
	linesA, stopA := a.start(t, pa)
	linesA.expect(t, "poller.tick")
	linesB, _ := b.start(t, pb)
	if skipped := linesB.expect(t, "poller.tick.skipped_locked"); skipped["poller"] != "test.contended" {
		t.Fatalf("skip line = %v, want the poller name", skipped)
	}
	for range 3 {
		clk.Advance(interval)
		linesA.expect(t, "poller.tick")
		linesB.expect(t, "poller.tick.skipped_locked")
	}
	if got := b.runner.LastTick("test.contended"); !got.Equal(clk.Now()) {
		t.Fatalf("follower LastTick = %v, want %v: a skipped attempt still proves the loop is alive", got, clk.Now())
	}
	if err := stopA(); err != nil {
		t.Fatalf("leader Run = %v, want nil after releasing its lock", err)
	}
	clk.Advance(interval)
	linesB.expect(t, "poller.tick")
	if na, nb := pa.ticks.Load(), pb.ticks.Load(); na != 4 || nb != 1 {
		t.Fatalf("ticks leader=%d follower=%d, want 4 and 1: one tick per interval across both runners", na, nb)
	}
}

func TestRunner_zeroChangeTickStillLogsWhatItScanned(t *testing.T) {
	t.Parallel()
	h := newHarness(t, testkit.DB(t), testkit.NewClock(epoch()))
	p := &fakePoller{name: "test.idle", tick: func(int32) (poller.Report, error) {
		return poller.Report{Scanned: 212, Attrs: []slog.Attr{slog.String("scope", "wallets")}}, nil
	}}
	if got := h.runner.LastTick("test.idle"); !got.IsZero() {
		t.Fatalf("LastTick before any tick = %v, want zero", got)
	}
	out, _ := h.start(t, p)
	line := out.expect(t, "poller.tick")
	want := map[string]any{"poller": "test.idle", "scanned": 212.0, "changed": 0.0, "duration_ms": 0.0}
	for k, v := range want {
		if line[k] != v {
			t.Errorf("poller.tick %s = %v, want %v (line %v)", k, line[k], v, line)
		}
	}
	if detail, _ := line["detail"].(map[string]any); detail["scope"] != "wallets" {
		t.Errorf("poller.tick detail = %v, want the report attrs", line["detail"])
	}
	if got := h.runner.LastTick("test.idle"); !got.Equal(epoch()) {
		t.Fatalf("LastTick = %v, want %v", got, epoch())
	}
}

func TestRunner_panickingTickIsRecoveredAndTheNextTickRuns(t *testing.T) {
	t.Parallel()
	h := newHarness(t, testkit.DB(t), testkit.NewClock(epoch()))
	p := &fakePoller{name: "test.panics", tick: func(n int32) (poller.Report, error) {
		if n == 1 {
			panic("boom")
		}
		return poller.Report{Changed: 1}, nil
	}}
	out, _ := h.start(t, p)
	failed := out.expect(t, "poller.tick.failed")
	detail, _ := failed["detail"].(map[string]any)
	stack, _ := detail["stack"].(string)
	if failed["code"] != "panic" || failed["alert"] != true || detail["panic"] != "boom" ||
		!strings.Contains(stack, "fakePoller") {
		t.Fatalf("failed line = %v, want code panic with the value and the stack", failed)
	}
	if got := h.errorCount(t, "test.panics", "panic"); got != 1 {
		t.Fatalf("poller_errors_total{panic} = %d, want 1", got)
	}
	h.clock.Advance(interval)
	if line := out.expect(t, "poller.tick"); line["changed"] != 1.0 {
		t.Fatalf("tick after the panic = %v, want changed 1", line)
	}
}

func TestRunner_tickErrorIsLoggedWithItsAttrsAndCountedByCode(t *testing.T) {
	t.Parallel()
	h := newHarness(t, testkit.DB(t), testkit.NewClock(epoch()))
	p := &fakePoller{name: "test.upstream", tick: func(int32) (poller.Report, error) {
		return poller.Report{}, errs.New(errs.CodeUpstreamUnavailable, "test.fetch", slog.String("provider", "rpc"))
	}}
	out, _ := h.start(t, p)
	for range 2 {
		failed := out.expect(t, "poller.tick.failed")
		detail, _ := failed["detail"].(map[string]any)
		if failed["poller"] != "test.upstream" || failed["code"] != "upstream_unavailable" ||
			failed["alert"] != false || detail["provider"] != "rpc" {
			t.Fatalf("failed line = %v, want the code and the error attrs", failed)
		}
		h.clock.Advance(interval)
	}
	out.expect(t, "poller.tick.failed")
	if got := h.errorCount(t, "test.upstream", "upstream_unavailable"); got != 3 {
		t.Fatalf("poller_errors_total{upstream_unavailable} = %d, want 3", got)
	}
}

func TestRunner_lockErrorIsCountedAndTheLoopKeepsGoing(t *testing.T) {
	t.Parallel()
	closed, err := pgxpool.NewWithConfig(t.Context(), testkit.DB(t).Config())
	if err != nil {
		t.Fatal(err)
	}
	closed.Close()
	h := newHarness(t, closed, testkit.NewClock(epoch()))
	p := &fakePoller{name: "test.nodb", tick: quiet}
	out, _ := h.start(t, p)
	out.expect(t, "poller.tick.failed")
	h.clock.Advance(interval)
	out.expect(t, "poller.tick.failed")
	if got := h.errorCount(t, "test.nodb", "internal"); got != 2 || p.ticks.Load() != 0 {
		t.Fatalf("errors = %d ticks = %d, want 2 errors and no tick without the lock", got, p.ticks.Load())
	}
}

func TestRunner_rejectsDuplicatePollerNames(t *testing.T) {
	t.Parallel()
	h := newHarness(t, nil, testkit.NewClock(epoch()))
	defer func() {
		if r := recover(); r != "poller.Runner.Run: duplicate poller test.twice" {
			t.Fatalf("recovered %v, want the duplicate-name panic", r)
		}
	}()
	err := h.runner.Run(t.Context(), &fakePoller{name: "test.twice"}, &fakePoller{name: "test.twice"})
	t.Fatalf("Run = %v, want a panic before any loop starts", err)
}

func TestRunner_stoppingMidTickIsNotAFailure(t *testing.T) {
	t.Parallel()
	h := newHarness(t, testkit.DB(t), testkit.NewClock(epoch()))
	started := make(chan struct{})
	p := &blockingPoller{fakePoller: fakePoller{name: "test.stopped"}, interval: interval, started: started}
	out, stop := h.start(t, p)
	<-started
	if err := stop(); err != nil {
		t.Fatalf("Run = %v, want nil", err)
	}
	if len(out) != 0 || h.errorCount(t, "test.stopped", "upstream_timeout") != 0 {
		t.Fatalf("stop mid-tick logged %v, want no line and no error count", <-out)
	}
}

func TestRunner_tickThatOutlivesItsIntervalIsCutOffAndCounted(t *testing.T) {
	t.Parallel()
	h := newHarness(t, testkit.DB(t), testkit.NewClock(epoch()))
	p := &blockingPoller{fakePoller: fakePoller{name: "test.slow"}, interval: 20 * time.Millisecond}
	out, _ := h.start(t, p)
	if failed := out.expect(t, "poller.tick.failed"); failed["code"] != "upstream_timeout" {
		t.Fatalf("failed line = %v, want the tick cut off at its interval", failed)
	}
	if got := h.errorCount(t, "test.slow", "upstream_timeout"); got != 1 {
		t.Fatalf("poller_errors_total{upstream_timeout} = %d, want 1", got)
	}
}

func TestRunner_refusesAPoolTooSmallForItsHeldLocks(t *testing.T) {
	t.Parallel()
	pool := poolWithMaxConns(t, testkit.DB(t), 2)
	h := newHarness(t, pool, testkit.NewClock(epoch()))
	defer func() {
		if r := recover(); r != "poller.Runner.Run: pool MaxConns 2 must exceed the 2 pollers that each hold a connection" {
			t.Fatalf("recovered %v, want the pool-size panic", r)
		}
	}()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := h.runner.Run(ctx, &fakePoller{name: "test.one", tick: quiet}, &fakePoller{name: "test.two", tick: quiet})
	t.Fatalf("Run = %v, want a panic before any loop starts", err)
}

func TestRunner_lockThatCannotBeTakenFailsWithinTheLockDeadline(t *testing.T) {
	t.Parallel()
	pool := poolWithMaxConns(t, testkit.DB(t), 2)
	for range 2 {
		conn, err := pool.Acquire(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(conn.Release)
	}
	h := newHarness(t, pool, testkit.NewClock(epoch()))
	p := &fakePoller{name: "test.starved", tick: quiet}
	out, _ := h.start(t, p)
	if failed := out.expect(t, "poller.tick.failed"); failed["code"] != "db_unavailable" {
		t.Fatalf("failed line = %v, want the lock attempt cut off as db_unavailable", failed)
	}
	if got := h.errorCount(t, "test.starved", "db_unavailable"); got != 1 || p.ticks.Load() != 0 {
		t.Fatalf("errors = %d ticks = %d, want 1 error and no tick", got, p.ticks.Load())
	}
}

func poolWithMaxConns(t *testing.T, base *pgxpool.Pool, maxConns int32) *pgxpool.Pool {
	t.Helper()
	cfg := base.Config()
	cfg.MaxConns = maxConns
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	return pool
}

type blockingPoller struct {
	fakePoller
	interval time.Duration
	started  chan struct{}
}

func (p *blockingPoller) Interval() time.Duration { return p.interval }

func (p *blockingPoller) Tick(ctx context.Context) (poller.Report, error) {
	if p.started != nil {
		close(p.started)
	}
	<-ctx.Done()
	return poller.Report{}, errs.Wrap(context.Cause(ctx), errs.CodeUpstreamTimeout, "test.slow")
}

type brokenMeter struct{ noop.Meter }

func (brokenMeter) Int64Counter(string, ...metric.Int64CounterOption) (metric.Int64Counter, error) {
	return nil, errs.New(errs.CodeInternal, "test.meter")
}

func TestNewRunner_failsWhenTheCounterCannotBeCreated(t *testing.T) {
	t.Parallel()
	runner, err := poller.NewRunner(nil, testkit.NewClock(epoch()), brokenMeter{})
	if runner != nil || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("NewRunner = %v, %v; want nil and internal", runner, err)
	}
}
