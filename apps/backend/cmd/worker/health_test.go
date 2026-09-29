package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type ticks map[string]time.Time

func (t ticks) LastTick(name string) time.Time { return t[name] }

type testPoller string

func (p testPoller) Name() string { return string(p) }

func (testPoller) Interval() time.Duration { return time.Minute }

func (testPoller) Tick(context.Context) (poller.Report, error) { return poller.Report{}, nil }

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
		ticks: ticks{"a.fresh": now, "b.edge": now.Add(-3*time.Minute + time.Nanosecond)},
	}
	want := []string{"nats ok", "db ok", "poller:a.fresh ok", "poller:b.edge ok"}
	if code, lines := get(h); code != http.StatusOK || !slices.Equal(lines, want) {
		t.Fatalf("healthy worker = %d %q, want 200 %q", code, lines, want)
	}

	pool.Close()
	h = health{
		connected: func() bool { return false }, pool: pool, clock: testkit.NewClock(now),
		pollers: append(pollers, testPoller("c.never")),
		ticks:   ticks{"a.fresh": now, "b.edge": now.Add(-3 * time.Minute)},
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
		pollers: []poller.Poller{testPoller("a.fresh")}, ticks: ticks{"a.fresh": now},
	}
	if code, lines := get(h); code != http.StatusServiceUnavailable || lines[0] != "nats disconnected" {
		t.Fatalf("worker with NATS down = %d %q, want 503 naming nats", code, lines)
	}
}
