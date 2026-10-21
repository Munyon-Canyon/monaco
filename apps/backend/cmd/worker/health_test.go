package main

import (
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const starvedDBPingLimit = 30 * time.Second

type ticks map[string]time.Time

func (t ticks) LastTick(name string) time.Time { return t[name] }

type testPoller string

func (p testPoller) Name() string { return string(p) }

func (testPoller) Interval() time.Duration { return time.Minute }

func (testPoller) Tick(context.Context) (poller.Report, error) { return poller.Report{}, nil }

type budgetedPoller struct{ testPoller }

func (budgetedPoller) Interval() time.Duration { return time.Second }

func (budgetedPoller) TickBudget() time.Duration { return 90 * time.Second }

func get(h health) (int, []string) {
	rec := httptest.NewRecorder()
	h.mux().ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/healthz", nil))
	return rec.Code, strings.Split(strings.TrimSuffix(rec.Body.String(), "\n"), "\n")
}

func TestHealth_isOKOnlyWhileNATSIsUpThePoolPingsAndEveryPollerTickedWithinThreeIntervals(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	pollers := make([]poller.Poller, 0, 3)
	pollers = append(pollers, testPoller("a.fresh"), testPoller("b.edge"))
	pool := testkit.DB(t)
	h := health{
		connected: func() bool { return true }, pool: pool, pollers: pollers, clock: testkit.NewClock(now),
		pingLimit: starvedDBPingLimit,
		ticks:     ticks{"a.fresh": now, "b.edge": now.Add(-3*time.Minute + time.Nanosecond)},
	}
	want := []string{"nats ok", "db ok", "poller:a.fresh ok", "poller:b.edge ok"}
	if code, lines := get(h); code != http.StatusOK || !slices.Equal(lines, want) {
		t.Fatalf("healthy worker = %d %q, want 200 %q", code, lines, want)
	}

	pool.Close()
	h = health{
		connected: func() bool { return false }, pool: pool, clock: testkit.NewClock(now),
		pingLimit: starvedDBPingLimit,
		pollers:   append(pollers, testPoller("c.never")),
		ticks:     ticks{"a.fresh": now, "b.edge": now.Add(-3 * time.Minute)},
	}
	want = []string{
		"nats disconnected", "db closed pool", "poller:a.fresh ok", "poller:b.edge stale: last tick 3m0s ago",
		"poller:c.never no tick yet",
	}
	if code, lines := get(h); code != http.StatusServiceUnavailable || !slices.Equal(lines, want) {
		t.Fatalf("failing worker = %d %q, want 503 %q", code, lines, want)
	}
}

func TestHealth_oneFailingCheckAloneMakesTheWorkerUnhealthy(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	h := health{
		connected: func() bool { return false }, pool: testkit.DB(t), clock: testkit.NewClock(now),
		pingLimit: starvedDBPingLimit,
		pollers:   []poller.Poller{testPoller("a.fresh")}, ticks: ticks{"a.fresh": now},
	}
	if code, lines := get(h); code != http.StatusServiceUnavailable || lines[0] != "nats disconnected" {
		t.Fatalf("worker with NATS down = %d %q, want 503 naming nats", code, lines)
	}
}

func TestHealth_aBudgetedPollerIsStaleOnlyAfterThreeTickBudgets(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	h := health{
		connected: func() bool { return true }, pool: testkit.DB(t), clock: testkit.NewClock(now),
		pingLimit: starvedDBPingLimit,
		pollers:   []poller.Poller{budgetedPoller{"a.long"}, budgetedPoller{"b.dead"}},
		ticks:     ticks{"a.long": now.Add(-89 * time.Second), "b.dead": now.Add(-270 * time.Second)},
	}
	want := []string{"nats ok", "db ok", "poller:a.long ok", "poller:b.dead stale: last tick 4m30s ago"}
	if code, lines := get(h); code != http.StatusServiceUnavailable || !slices.Equal(lines, want) {
		t.Fatalf("worker with a long tick = %d %q, want 503 %q", code, lines, want)
	}
}

func (t ticks) LastSuccess(name string) time.Time { return t[name] }

type stubProgress struct {
	p   progress
	err error
}

func (s stubProgress) Progress(context.Context) (progress, error) { return s.p, s.err }

func getReady(h health) (int, []string) {
	rec := httptest.NewRecorder()
	h.mux().ServeHTTP(rec, httptest.NewRequestWithContext(context.Background(), http.MethodGet, "/readyz", nil))
	return rec.Code, strings.Split(strings.TrimSuffix(rec.Body.String(), "\n"), "\n")
}

func TestReady_isOKWhileEveryMoneyPollerSucceededWithinThreeIntervalsAndReportsProgress(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	runs := ticks{"funding.withdrawals": now.Add(-3*time.Minute + time.Nanosecond), "ranking.valuation": now}
	h := health{
		connected: func() bool { return true }, pool: testkit.DB(t), clock: testkit.NewClock(now),
		pingLimit: starvedDBPingLimit, successes: runs,
		pollers: []poller.Poller{
			testPoller("funding.withdrawals"), testPoller("ranking.valuation"), testPoller("identity.nudges"),
		},
		progress: stubProgress{p: progress{
			MoneyIntentOldest: 95 * time.Second, OutboxUnpublished: 3, OutboxOldest: 4 * time.Second,
			ConsumerPending: 7, ConsumerOldest: 12 * time.Second,
		}},
	}
	want := []string{
		"nats ok", "db ok", "poller:funding.withdrawals ok", "poller:ranking.valuation ok",
		"progress:money_intent_oldest_seconds 95", "progress:outbox_unpublished 3",
		"progress:outbox_oldest_seconds 4", "progress:consumer_pending 7", "progress:consumer_oldest_seconds 12",
	}
	if code, lines := getReady(h); code != http.StatusOK || !slices.Equal(lines, want) {
		t.Fatalf("ready worker = %d %q, want 200 %q", code, lines, want)
	}
}

func TestReady_failsWhenAMoneyPollerKeepsFailingWhileLivenessReportsThePollerToo(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	h := health{
		connected: func() bool { return true }, pool: testkit.DB(t), clock: testkit.NewClock(now),
		pingLimit: starvedDBPingLimit,
		ticks:     ticks{"funding.withdrawals": now.Add(-3 * time.Minute)},
		successes: ticks{"funding.withdrawals": now.Add(-3 * time.Minute), "funding.deposit_watch": {}},
		pollers: []poller.Poller{
			testPoller("funding.withdrawals"), testPoller("funding.deposit_watch"), testPoller("market.prices"),
		},
	}
	want := []string{
		"nats ok", "db ok", "poller:funding.withdrawals stale: last success 3m0s ago",
		"poller:funding.deposit_watch no success yet",
	}
	if code, lines := getReady(h); code != http.StatusServiceUnavailable || !slices.Equal(lines, want) {
		t.Fatalf("stale worker = %d %q, want 503 %q", code, lines, want)
	}
	live := []string{
		"nats ok", "db ok", "poller:funding.withdrawals stale: last tick 3m0s ago",
		"poller:funding.deposit_watch no tick yet", "poller:market.prices no tick yet",
	}
	if code, lines := get(h); code != http.StatusServiceUnavailable || !slices.Equal(lines, live) {
		t.Fatalf("liveness of the same worker = %d %q, want 503 %q", code, lines, live)
	}
}

func TestReady_failsWhenProgressCannotBeRead(t *testing.T) {
	t.Parallel()
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)
	h := health{
		connected: func() bool { return true }, pool: testkit.DB(t), clock: testkit.NewClock(now),
		pingLimit: starvedDBPingLimit, successes: ticks{},
		progress: stubProgress{err: errors.New("relation withdrawals does not exist")},
	}
	want := []string{"nats ok", "db ok", "progress relation withdrawals does not exist"}
	if code, lines := getReady(h); code != http.StatusServiceUnavailable || !slices.Equal(lines, want) {
		t.Fatalf("worker without progress = %d %q, want 503 %q", code, lines, want)
	}
}

func TestReady_failsWhenThePoolIsExhausted(t *testing.T) {
	t.Parallel()
	cfg := testkit.DB(t).Config()
	cfg.MaxConns = 1
	pool, err := pgxpool.NewWithConfig(t.Context(), cfg)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(pool.Close)
	held, err := pool.Acquire(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(held.Release)
	h := health{
		connected: func() bool { return true }, pool: pool, clock: testkit.NewClock(time.Now()),
		pingLimit: 100 * time.Millisecond, successes: ticks{},
	}
	if code, lines := getReady(h); code != http.StatusServiceUnavailable || lines[1] != "db context deadline exceeded" {
		t.Fatalf("worker with an exhausted pool = %d %q, want 503 with the db check failing", code, lines)
	}
}
