package market_test

import (
	"bytes"
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/attribute"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type upstream struct {
	url, rpc     string
	catalogReads *atomic.Int32
	rpcCalls     *atomic.Int32
}

func fakeUpstream(t *testing.T, steps ...fakes.Step) upstream {
	t.Helper()
	srv := fakes.New()
	var catalogReads, rpcCalls atomic.Int32
	ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasPrefix(r.URL.Path, "/xstocks/"):
			catalogReads.Add(1)
		case strings.HasPrefix(r.URL.Path, "/rpc/"):
			rpcCalls.Add(1)
		}
		srv.ServeHTTP(w, r)
	}))
	t.Cleanup(ts.Close)
	for _, step := range steps {
		body, err := json.Marshal(step)
		if err != nil {
			t.Fatal(err)
		}
		req, err := http.NewRequestWithContext(t.Context(), http.MethodPost, ts.URL+"/_script", bytes.NewReader(body))
		if err != nil {
			t.Fatal(err)
		}
		resp, err := ts.Client().Do(req)
		if err != nil {
			t.Fatal(err)
		}
		_ = resp.Body.Close()
	}
	return upstream{url: ts.URL + "/xstocks", rpc: ts.URL + "/rpc/", catalogReads: &catalogReads, rpcCalls: &rpcCalls}
}

type worker struct {
	reader *sdkmetric.ManualReader

	mu    sync.Mutex
	lines []map[string]any
	read  int
}

func startWorker(t *testing.T, pool *pgxpool.Pool, clk *testkit.Clock, up upstream) *worker {
	t.Helper()
	ids := testkit.NewIDs(uint64(clk.Now().UnixNano()))
	cfg := moduleConfig()
	cfg.XStocks.BaseURL, cfg.Timeouts.XStocks = up.url, 10*time.Second
	cfg.Solana.RPCURL, cfg.Timeouts.RPC = up.rpc, 10*time.Second
	pollers := slices.DeleteFunc(market.New(module.Deps{
		Config: cfg, Clock: clk, IDs: ids, Pool: pool, UoW: db.New(pool, ids, clk), HTTPClient: httpclient.New,
	}).Pollers(), func(p poller.Poller) bool { return p.Name() != "market.catalog" })
	reader := sdkmetric.NewManualReader()
	runner, err := poller.NewRunner(pool, clk, sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader)).Meter("t"))
	if err != nil {
		t.Fatal(err)
	}
	w := &worker{reader: reader}
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

func (w *worker) Write(p []byte) (int, error) {
	var line map[string]any
	if err := json.Unmarshal(p, &line); err != nil {
		return 0, err
	}
	if msg, _ := line["msg"].(string); strings.HasPrefix(msg, "poller.") {
		w.mu.Lock()
		w.lines = append(w.lines, line)
		w.mu.Unlock()
	}
	return len(p), nil
}

func (w *worker) expect(t *testing.T, msg string) map[string]any {
	t.Helper()
	testkit.Eventually(t, func() bool {
		w.mu.Lock()
		defer w.mu.Unlock()
		return len(w.lines) > w.read
	}, 30*time.Second)
	w.mu.Lock()
	line := w.lines[w.read]
	w.read++
	w.mu.Unlock()
	if line["msg"] != msg || line["poller"] != "market.catalog" {
		t.Fatalf("next poller line = %v, want %s for market.catalog", line, msg)
	}
	return line
}

func (w *worker) errors(t *testing.T, code string) int64 {
	t.Helper()
	var rm metricdata.ResourceMetrics
	if err := w.reader.Collect(t.Context(), &rm); err != nil {
		t.Fatal(err)
	}
	want := attribute.NewSet(attribute.String("poller", "market.catalog"), attribute.String("code", code))
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

func TestCatalogPoller_twoWorkersOnOneDatabaseRunOneTickPerHour(t *testing.T) {
	t.Parallel()
	pool, clk := testkit.DB(t), testkit.NewClock(clock.Real{}.Now().UTC())
	up := fakeUpstream(t)
	a := startWorker(t, pool, clk, up)
	if tick := a.expect(t, "poller.tick"); tick["scanned"] != 3.0 || tick["changed"] != 6.0 {
		t.Fatalf("first tick = %v, want the three fixture assets inserted and checked against the chain", tick)
	}
	b := startWorker(t, pool, clk, up)
	b.expect(t, "poller.tick.skipped_locked")
	for range 2 {
		clk.Advance(time.Hour)
		a.expect(t, "poller.tick")
		b.expect(t, "poller.tick.skipped_locked")
	}
	if got := up.catalogReads.Load(); got != 3 {
		t.Fatalf("xStocks requests = %d, want 3: one catalog read per hour across both workers", got)
	}
	if got := up.rpcCalls.Load(); got != 1 {
		t.Fatalf("RPC calls = %d, want 1: one batch reads every unchecked fixture mint", got)
	}
	all, err := market.New(module.Deps{Pool: pool}).Catalog().ListAll(t.Context())
	if err != nil || len(all) != 3 || slices.ContainsFunc(all, func(a market.Asset) bool { return !a.ChainChecked }) {
		t.Fatalf("ListAll = %v, %v, want the three fixture assets, all checked", symbols(all), err)
	}
}

func TestCatalogPoller_providerFailureCountsInPollerErrors(t *testing.T) {
	t.Parallel()
	pool, clk := testkit.DB(t), testkit.NewClock(clock.Real{}.Now().UTC())
	up := fakeUpstream(t, fakes.Step{Route: "/xstocks/api/v2/public/assets", Action: fakes.ActionFail, Status: 404})
	w := startWorker(t, pool, clk, up)
	if failed := w.expect(t, "poller.tick.failed"); failed["code"] != "upstream_unavailable" {
		t.Fatalf("failed line = %v, want upstream_unavailable", failed)
	}
	if got := w.errors(t, "upstream_unavailable"); got != 1 {
		t.Fatalf("poller_errors_total{market.catalog, upstream_unavailable} = %d, want 1", got)
	}
	clk.Advance(time.Hour)
	if tick := w.expect(t, "poller.tick"); tick["scanned"] != 3.0 {
		t.Fatalf("recovered tick = %v, want the fixture catalog applied", tick)
	}
}

func TestCatalogPoller_anRPCFailureCountsInPollerErrorsAndTheNextTickChecksThatMint(t *testing.T) {
	t.Parallel()
	pool, clk := testkit.DB(t), testkit.NewClock(clock.Real{}.Now().UTC())
	up := fakeUpstream(t, fakes.Step{Route: "/rpc/getMultipleAccounts", Action: fakes.ActionFail, Status: 500})
	w := startWorker(t, pool, clk, up)
	if failed := w.expect(t, "poller.tick.failed"); failed["code"] != "rpc_unavailable" {
		t.Fatalf("failed line = %v, want rpc_unavailable", failed)
	}
	if got := w.errors(t, "rpc_unavailable"); got != 1 {
		t.Fatalf("poller_errors_total{market.catalog, rpc_unavailable} = %d, want 1", got)
	}
	clk.Advance(time.Hour)
	if tick := w.expect(t, "poller.tick"); tick["changed"] != 3.0 {
		t.Fatalf("retry tick = %v, want the batch of unchecked mints checked", tick)
	}
	all, err := market.New(module.Deps{Pool: pool}).Catalog().ListAll(t.Context())
	if err != nil || len(all) != 3 || slices.ContainsFunc(all, func(a market.Asset) bool { return !a.ChainChecked }) {
		t.Fatalf("ListAll = %v, %v, want the three fixture assets, all checked", all, err)
	}
}
