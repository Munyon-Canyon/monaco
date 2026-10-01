package market_test

import (
	"context"
	"encoding/json"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
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
	pool      *pgxpool.Pool
	clock     *testkit.Clock
	ids       *testkit.IDs
	providers app.Providers
	facts     *marketfake.MintFacts
	logs      *testkit.Logs
	catalog   *app.Catalog
	poller    *app.CatalogPoller
}

func newRig(t *testing.T, providers ...app.AssetProvider) *catalogRig {
	t.Helper()
	pool := testkit.DB(t)
	r := &catalogRig{
		pool: pool, clock: testkit.NewClock(clock.Real{}.Now().UTC().Truncate(time.Microsecond)),
		ids: testkit.NewIDs(7), providers: app.NewProviders(providers...), facts: &marketfake.MintFacts{},
		logs: &testkit.Logs{}, catalog: app.NewCatalog(pool),
	}
	for _, a := range marketfake.Fixtures() {
		r.facts.Put(a.Mint, a.Decimals, 1, 1)
	}
	r.poller = r.asking(r.facts)
	return r
}

func (r *catalogRig) asking(facts app.MintFacts) *app.CatalogPoller {
	return app.NewCatalogPoller(db.New(r.pool, r.ids, r.clock), r.pool, r.ids, r.clock, r.providers, facts)
}

func (r *catalogRig) ctx(t *testing.T) context.Context {
	t.Helper()
	return observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, r.logs))
}

func (r *catalogRig) tick(t *testing.T) poller.Report {
	t.Helper()
	report, err := r.poller.Tick(r.ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	return report
}

func (r *catalogRig) exec(t *testing.T, sql string, args ...any) {
	t.Helper()
	if _, err := r.pool.Exec(t.Context(), sql, args...); err != nil {
		t.Fatal(err)
	}
}

func (r *catalogRig) corrections(t *testing.T) []string {
	t.Helper()
	var out []string
	for line := range strings.Lines(string(r.logs.Bytes())) {
		var got map[string]any
		if json.Unmarshal([]byte(line), &got) != nil || got["msg"] != "market.catalog.decimals_corrected" {
			continue
		}
		b, err := json.Marshal([]any{
			got["level"], got["symbol"], got["mint"], got["issuer_decimals"],
			got["chain_decimals"],
		})
		if err != nil {
			t.Fatal(err)
		}
		out = append(out, string(b))
	}
	return out
}

func correction(a market.Asset, issuer, chain int) string {
	b, _ := json.Marshal([]any{"WARN", a.Symbol, a.Mint.String(), issuer, chain})
	return string(b)
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
	if got := rig.tick(t); got.Scanned != 3 || got.Changed != 4 {
		t.Fatalf("first tick = %+v, want 3 scanned, 2 inserted and 2 checked against the chain", got)
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

func TestCatalogPoller_refreshKeepsCheckedDecimalsAndMovesUncheckedOnesWithTheIssuer(t *testing.T) {
	t.Parallel()
	xs := &provider{issuer: domain.IssuerXStocks}
	aapl, tsla := listed(marketfake.AAPLx()), listed(marketfake.TSLAx())
	xs.serve(nil, aapl, tsla)
	rig := newRig(t, xs)
	rig.tick(t)
	rig.exec(t, `UPDATE assets SET decimals = 6 WHERE symbol = 'AAPLx'`)
	rig.exec(t, `UPDATE assets SET chain_checked_at = NULL WHERE symbol = 'TSLAx'`)
	aapl.Decimals, tsla.Decimals = 9, 9
	xs.serve(nil, aapl, tsla)
	rig.clock.Advance(time.Hour)
	if got := rig.tick(t); got.Changed != 2 {
		t.Fatalf("refresh = %+v, want the unchecked TSLAx moved to the issuer's 9 and then checked", got)
	}
	if a := rig.asset(t, "AAPLx"); a.Decimals != 6 || !a.ChainChecked || a.UpdatedAt.Equal(rig.clock.Now()) {
		t.Fatalf("checked AAPLx = %+v, want the chain's 6 decimals kept and the row untouched", a)
	}
	if a := rig.asset(t, "TSLAx"); a.Decimals != 8 || !a.ChainChecked {
		t.Fatalf("TSLAx = %+v, want the chain's 8 decimals once checked", a)
	}
	if got, want := rig.corrections(t), []string{correction(marketfake.TSLAx(), 9, 8)}; !slices.Equal(got, want) {
		t.Fatalf("corrections = %v, want %v: the issuer's refreshed 9 against the chain's 8", got, want)
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
	rig.facts.Put(spacex.Mint, spacex.Decimals, 1, 1)
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
	rig.facts.Put(spacex.Mint, spacex.Decimals, 1, 1)
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
