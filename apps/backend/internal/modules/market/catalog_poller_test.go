package market_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

type provider struct {
	issuer domain.Issuer

	mu     sync.Mutex
	assets []app.ProviderAsset
	err    error
}

func (p *provider) Issuer() domain.Issuer { return p.issuer }

func (p *provider) Catalog(context.Context) ([]app.ProviderAsset, error) {
	p.mu.Lock()
	defer p.mu.Unlock()
	return slices.Clone(p.assets), p.err
}

func (p *provider) serve(err error, assets ...app.ProviderAsset) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.assets, p.err = assets, err
}

func listed(a market.Asset) app.ProviderAsset {
	return app.ProviderAsset{
		Symbol: a.Symbol, Mint: a.Mint, Decimals: a.Decimals, Kind: a.Kind, DisplayName: a.DisplayName,
		LogoURL: a.LogoURL, Tradable: a.IssuerTradable,
	}
}

type catalogRig struct {
	pool    *pgxpool.Pool
	clock   *testkit.Clock
	catalog *app.Catalog
	poller  *app.CatalogPoller
}

func newRig(t *testing.T, providers ...app.AssetProvider) *catalogRig {
	t.Helper()
	pool := testkit.DB(t)
	clk := testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond))
	ids := testkit.NewIDs(7)
	return &catalogRig{
		pool: pool, clock: clk, catalog: app.NewCatalog(pool),
		poller: app.NewCatalogPoller(db.New(pool, ids, clk), ids, clk, app.NewProviders(providers...)),
	}
}

func (r *catalogRig) tick(t *testing.T) poller.Report {
	t.Helper()
	report, err := r.poller.Tick(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func (r *catalogRig) asset(t *testing.T, symbol string) market.Asset {
	t.Helper()
	a, err := r.catalog.AssetBySymbol(t.Context(), symbol)
	if err != nil {
		t.Fatal(err)
	}
	return a
}

func (r *catalogRig) tradable(t *testing.T) []string {
	t.Helper()
	all, err := r.catalog.ListTradable(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return symbols(all)
}

func TestCatalogPoller_InsertsAndUpdates(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	aapl, tsla := listed(marketfake.AAPLx()), listed(marketfake.TSLAx())
	xs.serve(nil, aapl, tsla, aapl)
	rig := newRig(t, xs)
	if rig.poller.Name() != "market.catalog" || rig.poller.Interval() != time.Hour {
		t.Fatalf("poller %s every %s, want market.catalog hourly", rig.poller.Name(), rig.poller.Interval())
	}
	inserted := rig.clock.Now()
	if got := rig.tick(t); got.Scanned != 3 || got.Changed != 2 {
		t.Fatalf("first tick = %+v, want 3 scanned and 2 inserted", got)
	}
	first := rig.asset(t, "AAPLx")
	want := marketfake.AAPLx()
	want.ID, want.FirstSeenAt, want.UpdatedAt = first.ID, first.FirstSeenAt, first.UpdatedAt
	if first != want || !first.FirstSeenAt.Equal(inserted) || !first.UpdatedAt.Equal(inserted) {
		t.Fatalf("inserted AAPLx = %+v, want %+v stamped %v", first, want, inserted)
	}
	rig.clock.Advance(time.Hour)
	if got := rig.tick(t); got.Changed != 0 {
		t.Fatalf("unchanged tick = %+v, want nothing changed", got)
	}
}

func TestCatalogPoller_updateKeepsTheIDAndFirstSeen(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	aapl, tsla := listed(marketfake.AAPLx()), listed(marketfake.TSLAx())
	xs.serve(nil, aapl, tsla)
	rig := newRig(t, xs)
	inserted := rig.clock.Now()
	rig.tick(t)
	first := rig.asset(t, "AAPLx")
	renamed := aapl
	renamed.DisplayName, renamed.LogoURL = "Apple Inc. xStock", ""
	xs.serve(nil, renamed, tsla)
	rig.clock.Advance(time.Hour)
	if got := rig.tick(t); got.Changed != 1 {
		t.Fatalf("rename tick = %+v, want one row changed", got)
	}
	updated := rig.asset(t, "AAPLx")
	if updated.ID != first.ID || !updated.FirstSeenAt.Equal(inserted) || !updated.UpdatedAt.Equal(rig.clock.Now()) ||
		updated.DisplayName != "Apple Inc. xStock" || updated.CompanyKey != "apple inc." || updated.LogoURL != "" {
		t.Fatalf("updated AAPLx = %+v, want the new name, the first ID and first_seen_at, updated now", updated)
	}
	if tsla := rig.asset(t, "TSLAx"); !tsla.UpdatedAt.Equal(inserted) || tsla.PopularRank != 7 {
		t.Fatalf("TSLAx = %+v, want untouched since the first tick with popular rank 7", tsla)
	}
}

func TestCatalogPoller_PausedMintTurnsUntradable(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	aapl, tsla := listed(marketfake.AAPLx()), listed(marketfake.TSLAx())
	xs.serve(nil, aapl, tsla)
	rig := newRig(t, xs)
	rig.tick(t)
	aapl.Tradable = false
	xs.serve(nil, aapl, tsla)
	rig.tick(t)
	if got := rig.tradable(t); !slices.Equal(got, []string{"TSLAx"}) {
		t.Fatalf("tradable = %v, want only TSLAx after AAPLx paused", got)
	}
	if a := rig.asset(t, "AAPLx"); a.IssuerTradable || a.Tradable() {
		t.Fatalf("paused AAPLx = %+v, want untradable", a)
	}
}

func TestCatalogPoller_DelistedMintTurnsUntradable(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	tessera := &provider{issuer: domain.IssuerTessera}
	spacex := listed(marketfake.JPSTx())
	spacex.Symbol, spacex.Tradable, spacex.Kind = "SPACEX", true, domain.KindPreIPO
	spacex.Mint = mint(t, "So11111111111111111111111111111111111111112")
	xs.serve(nil, listed(marketfake.AAPLx()), listed(marketfake.TSLAx()))
	tessera.serve(nil, spacex)
	rig := newRig(t, xs, tessera)
	rig.tick(t)
	xs.serve(nil, listed(marketfake.TSLAx()))
	if got := rig.tick(t); got.Scanned != 2 || got.Changed != 1 {
		t.Fatalf("delist tick = %+v, want 2 scanned and AAPLx changed", got)
	}
	if got := rig.tradable(t); !slices.Equal(got, []string{"TSLAx", "SPACEX"}) {
		t.Fatalf("tradable = %v, want TSLAx and the other issuer's SPACEX", got)
	}
	if a := rig.asset(t, "AAPLx"); a.IssuerTradable || !a.UpdatedAt.Equal(rig.clock.Now()) {
		t.Fatalf("delisted AAPLx = %+v, want untradable and updated now", a)
	}
}

func TestCatalogPoller_ProviderFailureKeepsRows(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	tessera := &provider{issuer: domain.IssuerTessera}
	spacex := listed(marketfake.JPSTx())
	spacex.Symbol, spacex.Tradable = "SPACEX", true
	spacex.Mint = mint(t, "So11111111111111111111111111111111111111112")
	xs.serve(nil, listed(marketfake.AAPLx()))
	tessera.serve(nil, spacex)
	rig := newRig(t, xs, tessera)
	rig.tick(t)
	before := rig.asset(t, "SPACEX")
	for name, fault := range map[string]func(){
		"error":         func() { tessera.serve(errs.New(errs.CodeUpstreamTimeout, "test")) },
		"empty catalog": func() { tessera.serve(nil) },
	} {
		fault()
		xs.serve(nil, listed(marketfake.AAPLx()), listed(marketfake.TSLAx()))
		rig.clock.Advance(time.Hour)
		report, err := rig.poller.Tick(t.Context())
		if err == nil || report.Scanned != 2 {
			t.Fatalf("%s: Tick = %+v, %v, want an error after xstocks applied", name, report, err)
		}
		if after := rig.asset(t, "SPACEX"); after != before {
			t.Fatalf("%s: SPACEX = %+v, want untouched %+v", name, after, before)
		}
		if got := rig.tradable(t); !slices.Equal(got, []string{"AAPLx", "TSLAx", "SPACEX"}) {
			t.Fatalf("%s: tradable = %v, want the healthy issuer applied and SPACEX kept", name, got)
		}
	}
}

func TestCatalogPoller_symbolReusedUnderANewMintFailsThatIssuerOnly(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	aapl := listed(marketfake.AAPLx())
	xs.serve(nil, aapl)
	rig := newRig(t, xs)
	rig.tick(t)
	aapl.Mint = mint(t, "So11111111111111111111111111111111111111112")
	xs.serve(nil, aapl)
	if _, err := rig.poller.Tick(t.Context()); err == nil {
		t.Fatal("Tick with AAPLx under a second mint succeeded, want the unique symbol to refuse it")
	}
	if a := rig.asset(t, "AAPLx"); a.Mint != marketfake.AAPLx().Mint || !a.IssuerTradable {
		t.Fatalf("AAPLx = %+v, want the first mint kept tradable", a)
	}
}

func TestCatalogPoller_cancelledTickFails(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	rig := newRig(t, xs)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := rig.poller.Tick(ctx); err == nil {
		t.Fatal("Tick on a cancelled context succeeded")
	}
}

func TestProviders_panicOnADuplicateIssuer(t *testing.T) {
	t.Parallel()
	defer func() {
		if got := recover(); got != "market: issuer xstocks registered twice" {
			t.Fatalf("recover = %v, want the duplicate issuer named", got)
		}
	}()
	app.NewProviders(&provider{issuer: domain.IssuerXStocks}, &provider{issuer: domain.IssuerXStocks})
}

func mint(t *testing.T, raw string) domain.Mint {
	t.Helper()
	m, err := domain.ParseMint(raw)
	if err != nil {
		t.Fatal(err)
	}
	return m
}
