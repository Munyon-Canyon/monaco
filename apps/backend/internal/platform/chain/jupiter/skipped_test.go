package jupiter_test

import (
	"bytes"
	"log/slog"
	"strings"
	"sync"
	"testing"
	"time"

	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func unpricedGauge(t *testing.T, c *jupiter.Client) int64 {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	c.ObserveUnpriced(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("test"))
	var rm metricdata.ResourceMetrics
	if err := reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	for _, scope := range rm.ScopeMetrics {
		for _, m := range scope.Metrics {
			if g, ok := m.Data.(metricdata.Gauge[int64]); ok && m.Name == "monaco_jupiter_unpriced_mints" {
				return g.DataPoints[0].Value
			}
		}
	}
	t.Fatal("monaco_jupiter_unpriced_mints not reported")
	return 0
}

type steppedClock struct {
	clock.Real
	mu sync.Mutex
	at time.Time
}

func (c *steppedClock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.at
}

func (c *steppedClock) advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.at = c.at.Add(d)
}

func TestPrices_anUnpricedMintLogsOncePerHourAndIsCounted(t *testing.T) {
	t.Parallel()
	unpriced, priced := fixtureMint(1), fixtureMint(2)
	body := `{"` + unpriced.Address + `":{"decimals":8},"` + priced.Address + `":{"usdPrice":1.5}}`
	clk := &steppedClock{at: time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)}
	c := jupiter.New(testConfig(), clk, httpclient.WithTransport(replying(200, body)))
	var logs bytes.Buffer
	ctx := observability.WithLogger(t.Context(), slog.New(slog.NewTextHandler(&logs, nil)))
	lines := func() int { return strings.Count(logs.String(), "msg=jupiter.price_skipped") }
	poll := func() {
		t.Helper()
		if _, err := c.Prices(ctx, []jupiter.Mint{unpriced, priced}); err != nil {
			t.Fatal(err)
		}
	}
	poll()
	clk.advance(2 * time.Minute)
	poll()
	clk.advance(57 * time.Minute)
	poll()
	if lines() != 1 {
		t.Fatalf("%d price_skipped lines inside the hour, want 1:\n%s", lines(), logs.String())
	}
	clk.advance(time.Minute)
	poll()
	if lines() != 2 {
		t.Fatalf("%d price_skipped lines after the hour, want 2", lines())
	}
	if got := unpricedGauge(t, c); got != 1 {
		t.Fatalf("UnpricedMints = %d, want 1", got)
	}
}

func TestPrices_aMintThatGetsAPriceLeavesTheUnpricedCount(t *testing.T) {
	t.Parallel()
	m := fixtureMint(1)
	answer := `{"` + m.Address + `":{"decimals":8}}`
	var mu sync.Mutex
	u := replyingWith(func() string { mu.Lock(); defer mu.Unlock(); return answer })
	c := jupiter.New(testConfig(), clock.Real{}, httpclient.WithTransport(u))
	if _, err := c.Prices(t.Context(), []jupiter.Mint{m}); err != nil {
		t.Fatal(err)
	}
	if got := unpricedGauge(t, c); got != 1 {
		t.Fatalf("UnpricedMints = %d, want 1", got)
	}
	mu.Lock()
	answer = `{"` + m.Address + `":{"usdPrice":2}}`
	mu.Unlock()
	if _, err := c.Prices(t.Context(), []jupiter.Mint{m}); err != nil {
		t.Fatal(err)
	}
	if got := unpricedGauge(t, c); got != 0 {
		t.Fatalf("UnpricedMints = %d after the price arrived, want 0", got)
	}
}
