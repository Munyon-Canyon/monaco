package market_test

import (
	"maps"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

func listAll(t *testing.T, pool *pgxpool.Pool) map[string]market.Asset {
	t.Helper()
	all, err := market.New(module.Deps{Pool: pool}).Catalog().ListAll(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	out := make(map[string]market.Asset, len(all))
	for _, a := range all {
		out[a.Symbol] = a
	}
	return out
}

func countByIssuer(assets map[string]market.Asset) map[domain.Issuer]int {
	out := map[domain.Issuer]int{}
	for _, a := range assets {
		out[a.Issuer]++
	}
	return out
}

func TestCatalogPoller_ThreeProviders(t *testing.T) {
	t.Parallel()
	pool, clk := testkit.DB(t), testkit.NewClock(clock.Real{}.Now().UTC())
	up := fakeUpstream(t, fakes.Step{Route: "/tessera/v1/public/token-details", Action: fakes.ActionFail, Status: 404})
	w := startWorker(t, pool, clk, up)
	if failed := w.expect(t, "poller.tick.failed"); failed["code"] != "upstream_unavailable" {
		t.Fatalf("failed line = %v, want upstream_unavailable from Tessera", failed)
	}
	got := countByIssuer(listAll(t, pool))
	if want := map[domain.Issuer]int{domain.IssuerXStocks: 3, domain.IssuerPreStocks: 8}; !maps.Equal(got, want) {
		t.Fatalf("rows by issuer with Tessera down = %v, want %v", got, want)
	}
	clk.Advance(time.Hour)
	if tick := w.expect(t, "poller.tick"); tick["scanned"] != fixtureAssets {
		t.Fatalf("recovered tick = %v, want every fixture asset", tick)
	}
	assets := listAll(t, pool)
	got = countByIssuer(assets)
	want := map[domain.Issuer]int{domain.IssuerXStocks: 3, domain.IssuerTessera: 3, domain.IssuerPreStocks: 8}
	if !maps.Equal(got, want) {
		t.Fatalf("rows by issuer = %v, want %v", got, want)
	}
	assertNoIssuerBranding(t, assets)
	tessera := assertOneSpaceX(t, assets)
	siblings, err := market.New(module.Deps{Pool: pool}).Catalog().Siblings(t.Context(), tessera.ID)
	if err != nil || len(siblings) != 1 || siblings[0].Symbol != "SPACEX" {
		t.Fatalf("Siblings(tSpaceX) = %+v, %v, want SPACEX", siblings, err)
	}
}

func assertNoIssuerBranding(t *testing.T, assets map[string]market.Asset) {
	t.Helper()
	for symbol, a := range assets {
		name := strings.ToLower(a.DisplayName)
		if strings.Contains(name, "xstock") || strings.HasPrefix(name, "t-") || strings.Contains(name, "prestocks") {
			t.Fatalf("%s display_name = %q, want no issuer branding", symbol, a.DisplayName)
		}
	}
}

func assertOneSpaceX(t *testing.T, assets map[string]market.Asset) market.Asset {
	t.Helper()
	tessera, prestocks := assets["tSpaceX"], assets["SPACEX"]
	for _, a := range []market.Asset{tessera, prestocks} {
		if a.DisplayName != "SpaceX" || a.CompanyKey != "spacex" || a.Kind != domain.KindPreIPO || a.Decimals != 9 {
			t.Fatalf("%s = %+v, want a 9-decimal pre-IPO SpaceX under company key spacex", a.Symbol, a)
		}
	}
	return tessera
}

func TestPreStocksMultiplier(t *testing.T) {
	t.Parallel()
	pool, clk := testkit.DB(t), testkit.NewClock(clock.Real{}.Now().UTC())
	w := startWorker(t, pool, clk, fakeUpstream(t))
	w.expect(t, "poller.tick")
	assets := listAll(t, pool)
	for symbol, want := range map[string]domain.Multiplier{
		"SPACEX":  {Num: 5, Den: 1},
		"OPENAI":  {Num: 14861347, Den: 10000000},
		"ANDURIL": {Num: 1, Den: 1},
		"tSpaceX": {Num: 1, Den: 1},
	} {
		a, ok := assets[symbol]
		if !ok || !a.ChainChecked {
			t.Fatalf("%s = %+v, want a chain-checked row", symbol, a)
		}
		if got := a.UIMultiplierAt(clk.Now()); got != want {
			t.Fatalf("%s multiplier = %+v, want %+v", symbol, got, want)
		}
	}
	if symbols := slices.Sorted(maps.Keys(assets)); len(symbols) != fixtureAssets {
		t.Fatalf("symbols = %v, want every fixture asset", symbols)
	}
}
