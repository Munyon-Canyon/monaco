package market_test

import (
	"context"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestPriceHint_ForwardsTick(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	reader := sdkmetric.NewManualReader()
	provider := sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))
	clk := testkit.NewClock(time.Date(2026, 10, 2, 15, 0, 0, 0, time.UTC))
	unsub, err := market.New(module.Deps{Bus: b.Conn, Clock: clk}).PriceHints(t.Context(), provider.Meter("market"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(unsub)
	subject := b.Conn.Subject("price.tick")
	if n := testkit.NATSSubscriptions(t, subject); n != 1 {
		t.Fatalf("subscriptions on %s = %d, want 1", subject, n)
	}
	got := make(chan string, 1)
	if err := b.Conn.SubscribeHints(t.Context(), func(_ context.Context, key string) { got <- key }); err != nil {
		t.Fatal(err)
	}
	if err := b.Conn.PublishCore(t.Context(), events.PriceTick{V: 1}); err != nil {
		t.Fatal(err)
	}
	var key string
	testkit.Eventually(t, func() bool {
		select {
		case key = <-got:
			return true
		default:
			return false
		}
	}, 5*time.Second)
	if key != "global.prices_updated" {
		t.Fatalf("hint = %q, want global.prices_updated", key)
	}
	hub, err := sse.NewHub(sse.NoMemberships{}, provider)
	if err != nil {
		t.Fatal(err)
	}
	hub.Deliver(t.Context(), key)
	if n := metricTotal(t, reader, "monaco_sse_unknown_subjects_total"); n != 0 {
		t.Fatalf("monaco_sse_unknown_subjects_total = %d, want 0", n)
	}
	if n := metricTotal(t, reader, "monaco_sse_hints_total"); n != 1 {
		t.Fatalf("monaco_sse_hints_total = %d, want 1", n)
	}
	if n := metricTotal(t, reader, "market_price_hints_total"); n != 1 {
		t.Fatalf("market_price_hints_total = %d, want 1", n)
	}
	unsub()
	testkit.Eventually(t, func() bool { return testkit.NATSSubscriptions(t, subject) == 0 }, 5*time.Second)
}

func TestModule_PriceHintsFailsWhenTheCounterCannotBeCreated(t *testing.T) {
	t.Parallel()
	meter := testkit.FailingGauges{Prefix: "market_price_hints"}.Meter("market")
	_, err := market.New(module.Deps{}).PriceHints(t.Context(), meter)
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("PriceHints = %v, want internal", err)
	}
}

func metricTotal(t *testing.T, reader *sdkmetric.ManualReader, name string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	var total int64
	for _, sm := range rm.ScopeMetrics {
		for _, m := range sm.Metrics {
			sum, ok := m.Data.(metricdata.Sum[int64])
			if !ok || m.Name != name {
				continue
			}
			for _, p := range sum.DataPoints {
				total += p.Value
			}
		}
	}
	return total
}
