package poller_test

import (
	"context"
	"encoding/json"
	"log/slog"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const interval = time.Minute

func epoch() time.Time { return time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC) }

type fakePoller struct {
	name  string
	ticks atomic.Int32
	tick  func(n int32) (poller.Report, error)
}

func (p *fakePoller) Name() string { return p.name }

func (*fakePoller) Interval() time.Duration { return interval }

func (p *fakePoller) Tick(context.Context) (poller.Report, error) {
	return p.tick(p.ticks.Add(1))
}

func quiet(int32) (poller.Report, error) { return poller.Report{}, nil }

type lines chan map[string]any

func (l lines) Write(p []byte) (int, error) {
	var line map[string]any
	if err := json.Unmarshal(p, &line); err != nil {
		return 0, err
	}
	l <- line
	return len(p), nil
}

func (l lines) expect(t *testing.T, msg string) map[string]any {
	t.Helper()
	select {
	case line := <-l:
		if line["msg"] != msg {
			t.Fatalf("next log line = %v, want %s", line, msg)
		}
		return line
	case <-time.After(30 * time.Second):
		t.Fatalf("no %s line within 30s", msg)
		return nil
	}
}

type harness struct {
	pool   *pgxpool.Pool
	clock  *testkit.Clock
	reader *sdkmetric.ManualReader
	runner *poller.Runner
}

func newHarness(t *testing.T, pool *pgxpool.Pool, clk *testkit.Clock) *harness {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("poller_test")
	runner, err := poller.NewRunner(pool, clk, meter)
	if err != nil {
		t.Fatal(err)
	}
	return &harness{pool: pool, clock: clk, reader: reader, runner: runner}
}

func (h *harness) start(t *testing.T, pollers ...poller.Poller) (lines, func() error) {
	t.Helper()
	out := make(lines, 64)
	ctx, cancel := context.WithCancel(context.WithoutCancel(t.Context()))
	ctx = observability.WithLogger(
		ctx,
		slog.New(slog.NewJSONHandler(out, &slog.HandlerOptions{Level: slog.LevelDebug})),
	)
	done := make(chan error, 1)
	go func() { done <- h.runner.Run(ctx, pollers...) }()
	stop := sync.OnceValue(func() error {
		cancel()
		return <-done
	})
	t.Cleanup(func() {
		if err := stop(); err != nil {
			t.Errorf("Run returned %v at cleanup", err)
		}
	})
	return out, stop
}

func (h *harness) errorCount(t *testing.T, name, code string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := h.reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	want := attribute.NewSet(attribute.String("poller", name), attribute.String("code", code))
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || m.Name != "poller_errors_total" {
				continue
			}
			for _, dp := range sum.DataPoints {
				if dp.Attributes.Equals(&want) {
					total += dp.Value
				}
			}
		}
	}
	return total
}
