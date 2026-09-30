package market_test

import (
	"encoding/json"
	"net/http"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type flow18 struct {
	pool   *pgxpool.Pool
	clock  *testkit.Clock
	bucket time.Time
	bus    testkit.Bus
	ticks  *testkit.CoreSubscription
	poller poller.Poller
}

func startFlow18(t *testing.T, quoteTimeout time.Duration, steps ...fakes.Step) *flow18 {
	t.Helper()
	pool := testkit.DB(t)
	bucket := clock.Real{}.Now().UTC().Truncate(domain.SampleBucket)
	seedAssets(t, pool, bucket, marketfake.Fixtures()...)
	j := fakeJupiter(t, steps...)
	cfg := moduleConfig()
	cfg.Jupiter.SwapBaseURL, cfg.Jupiter.PriceBaseURL = j.url+"/jupiter/swap/v2", j.url+jupiterPriceRoute
	cfg.Timeouts.JupiterQuote = quoteTimeout
	clk, ids, b := testkit.NewClock(bucket.Add(37*time.Second)), testkit.NewIDs(18), testkit.NATS(t)
	f := &flow18{
		pool:   pool,
		clock:  clk,
		bucket: bucket,
		bus:    b,
		ticks:  testkit.SubscribeCore(t, b, string(events.TypePriceTick)),
	}
	deps := module.Deps{
		Config: cfg, Clock: clk, IDs: ids, Pool: pool, UoW: db.New(pool, ids, clk), Bus: b.Conn,
		HTTPClient: httpclient.New,
	}
	for _, p := range market.New(deps).Pollers() {
		if p.Name() == "market.prices" {
			f.poller = p
		}
	}
	return f
}

func (f *flow18) published(t *testing.T) []events.PriceTick {
	t.Helper()
	if err := f.bus.Conn.PublishCore(t.Context(), events.PriceTick{}); err != nil {
		t.Fatal(err)
	}
	var ticks []events.PriceTick
	for {
		var tick events.PriceTick
		if err := json.Unmarshal(f.ticks.Next(t), &tick); err != nil {
			t.Fatal(err)
		}
		if tick.V == 0 {
			return ticks
		}
		ticks = append(ticks, tick)
	}
}

func catalogPrices() fakes.Step {
	return fakes.Step{Route: jupiterPriceRoute, Action: fakes.ActionSucceed, Fixture: jupiterPriceRoute + "/catalog"}
}

func TestFlow18_SamplePrices_OK(t *testing.T) {
	t.Parallel()
	f := startFlow18(t, 5*time.Second, catalogPrices())
	aapl, tsla := marketfake.AAPLx(), marketfake.TSLAx()
	report, err := f.poller.Tick(t.Context())
	if err != nil || report.Scanned != 3 || report.Changed != 2 || attr(report.Attrs, "missing") != "1" {
		t.Fatalf("Tick = %+v, %v, want the 3 catalog mints scanned, 2 written and JPSTx missing", report, err)
	}
	wantPoints(t, f.pool,
		pricePoint{aapl.Mint.String(), f.bucket, 254_371_234, "jupiter"},
		pricePoint{tsla.Mint.String(), f.bucket, 436_120_500, "jupiter"},
	)
	ticks := f.published(t)
	if len(ticks) != 1 {
		t.Fatalf("price.tick messages = %d, want 1 for the tick", len(ticks))
	}
	wantTick(t, ticks[0], f.bucket, tickPrice(aapl, 254_371_234, f.bucket), tickPrice(tsla, 436_120_500, f.bucket))
}

func TestFlow18_SamplePrices_JupiterUnavailable(t *testing.T) {
	t.Parallel()
	f := startFlow18(t, 5*time.Second, catalogPrices(),
		fakes.Step{Route: jupiterPriceRoute, Action: fakes.ActionFail, Status: http.StatusInternalServerError})
	aapl, tsla := marketfake.AAPLx(), marketfake.TSLAx()
	if _, err := f.poller.Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(domain.SampleBucket)
	_, err := f.poller.Tick(t.Context())
	if detail := errs.Detail(err); errs.CodeOf(err) != errs.CodeJupiterUnavailable ||
		attr(detail, "written") != "0" || attr(detail, "missing") != "3" {
		t.Fatalf("Tick while Jupiter answers 500 = %v %v, want jupiter_unavailable with nothing written", err, detail)
	}
	wantPoints(t, f.pool,
		pricePoint{aapl.Mint.String(), f.bucket, 254_371_234, "jupiter"},
		pricePoint{tsla.Mint.String(), f.bucket, 436_120_500, "jupiter"},
	)
	ticks := f.published(t)
	if len(ticks) != 2 {
		t.Fatalf("price.tick messages = %d, want 1 per tick", len(ticks))
	}
	wantTick(t, ticks[1], f.bucket.Add(domain.SampleBucket),
		tickPrice(aapl, 254_371_234, f.bucket), tickPrice(tsla, 436_120_500, f.bucket))
}

func TestFlow18_SamplePrices_UpstreamTimeout(t *testing.T) {
	t.Parallel()
	f := startFlow18(t, 200*time.Millisecond, fakes.Step{Route: jupiterPriceRoute, Action: fakes.ActionHang})
	if _, err := f.poller.Tick(t.Context()); errs.CodeOf(err) != errs.CodeUpstreamTimeout {
		t.Fatalf("Tick while Jupiter hangs = %v, want upstream_timeout", err)
	}
	wantPoints(t, f.pool)
	ticks := f.published(t)
	if len(ticks) != 1 {
		t.Fatalf("price.tick messages = %d, want 1 for the tick", len(ticks))
	}
	wantTick(t, ticks[0], f.bucket)
}
