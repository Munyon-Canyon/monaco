package app

import (
	"context"
	"sync"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestPriceHint_Coalesces(t *testing.T) {
	t.Parallel()
	url := testkit.StandaloneNATS(t)
	conn, err := bus.Connect(t.Context(), config.NATS{URL: url}, bus.ProcessWorker)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(t.Context()) })
	reader := sdkmetric.NewManualReader()
	meter := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("market")
	clk := testkit.NewClock(time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC))
	hints, err := NewPriceHints(conn, clk, meter)
	if err != nil {
		t.Fatal(err)
	}
	var mu sync.Mutex
	var keys []string
	if err := conn.SubscribeHints(t.Context(), func(_ context.Context, key string) {
		mu.Lock()
		keys = append(keys, key)
		mu.Unlock()
	}); err != nil {
		t.Fatal(err)
	}

	hints.note(t.Context())
	clk.Advance(10 * time.Second)
	hints.note(t.Context())
	if n := hintTotal(t, reader); n != 1 {
		t.Fatalf("market_price_hints_total = %d after two ticks 10s apart, want 1", n)
	}
	clk.Advance(40 * time.Second)
	hints.note(t.Context())
	if n := hintTotal(t, reader); n != 2 {
		t.Fatalf("market_price_hints_total = %d after a tick 40s later, want 2", n)
	}
	testkit.Eventually(t, func() bool {
		mu.Lock()
		defer mu.Unlock()
		return len(keys) >= 2
	}, 5*time.Second)
	mu.Lock()
	defer mu.Unlock()
	if len(keys) != 2 || keys[0] != priceHintKey || keys[1] != priceHintKey {
		t.Fatalf("hints = %v, want two %s", keys, priceHintKey)
	}
}

func TestNewPriceHints_failsWhenTheCounterCannotBeCreated(t *testing.T) {
	t.Parallel()
	meter := testkit.FailingGauges{Prefix: "market_price_hints"}.Meter("market")
	_, err := NewPriceHints(nil, testkit.NewClock(time.Unix(0, 0).UTC()), meter)
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("NewPriceHints = %v, want internal", err)
	}
}

func hintTotal(t *testing.T, reader *sdkmetric.ManualReader) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || m.Name != "market_price_hints_total" {
				continue
			}
			for _, p := range sum.DataPoints {
				total += p.Value
			}
		}
	}
	return total
}
