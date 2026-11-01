package treasury_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestTopHoldings_picksEachCabalsLargestCostBasisAmongHeldAssetsWithoutPrices(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	first, second, empty := f.cabal(t), f.cabal(t), f.cabal(t)
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, $2, $3, $4, $5)`
	for _, row := range []struct {
		cabal ids.CabalID
		mint  string
		units int64
		cost  int64
	}{
		{first, "MintSmall", 5, 100},
		{first, "MintLarge", 1, 900},
		{first, "MintSoldOut", 0, 5000},
		{second, "MintB", 2, 300},
		{second, "MintA", 2, 300},
		{empty, "MintGone", 0, 700},
	} {
		if _, err := f.pool.Exec(t.Context(), insert, row.cabal.UUID(), row.mint, row.units, row.cost,
			f.clock.Now()); err != nil {
			t.Fatal(err)
		}
	}
	got, err := treasury.New(module.Deps{Pool: f.pool}).TopHoldings().
		TopHoldings(t.Context(), []ids.CabalID{first, second, empty})
	want := map[ids.CabalID]chain.SolanaAddress{first: "MintLarge", second: "MintA"}
	if err != nil || len(got) != len(want) || got[first] != want[first] || got[second] != want[second] {
		t.Fatalf("TopHoldings = %v, %v, want %v with no entry for the cabal without a held position", got, err, want)
	}
	if none, err := treasury.New(module.Deps{Pool: f.pool}).TopHoldings().TopHoldings(t.Context(), nil); err != nil ||
		len(none) != 0 {
		t.Fatalf("TopHoldings(nil) = %v, %v, want empty", none, err)
	}
}

func TestTopHoldings_wrapsStoreFailures(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	got, err := treasury.New(module.Deps{Pool: f.pool}).TopHoldings().TopHoldings(ctx, []ids.CabalID{f.cabal(t)})
	if got != nil || errs.CodeOf(err) != errs.CodeDBUnavailable {
		t.Fatalf("TopHoldings on a cancelled context = %v, %v, want a db_unavailable error", got, err)
	}
}

func (f *fixture) listAsset(t *testing.T, mint string, decimals int) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO assets (id, symbol, mint, decimals, issuer, kind, display_name,
		issuer_tradable, company_key, first_seen_at, updated_at)
		VALUES ($1, $2, $2, $3, 'xstocks', 'equity', $2, true, $2, now(), now())`,
		f.ids.NewV7(), mint, decimals); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) pricePoint(t *testing.T, mint string, micros int64, age time.Duration) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO price_points (mint, ts, price_micros, source)
		VALUES ($1, now() - $2::interval, $3, 'test')`, mint, fmt.Sprintf("%d seconds", int(age.Seconds())), micros); err != nil {
		t.Fatal(err)
	}
}

func (f *fixture) position(t *testing.T, cabal ids.CabalID, mint string, cost int64) {
	t.Helper()
	const oneToken = 1_000_000
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO cabal_positions
		(cabal_id, asset, units, cost_basis_micros, updated_at) VALUES ($1, $2, $3, $4, now())`,
		cabal.UUID(), mint, oneToken, cost); err != nil {
		t.Fatal(err)
	}
}

func TestTopHoldings_ranksByCurrentValueAndFallsBackToCostBasisWithoutAFreshPrice(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	risen, stale := f.cabal(t), f.cabal(t)
	for _, mint := range []string{"MintTripled", "MintFell", "MintStaleBig", "MintFreshSmall"} {
		f.listAsset(t, mint, 6)
	}
	f.position(t, risen, "MintTripled", 100_000_000)
	f.pricePoint(t, "MintTripled", 300_000_000, time.Minute)
	f.position(t, risen, "MintFell", 200_000_000)
	f.pricePoint(t, "MintFell", 50_000_000, time.Minute)
	f.position(t, stale, "MintStaleBig", 900_000_000)
	f.pricePoint(t, "MintStaleBig", 1_000_000, time.Hour)
	f.position(t, stale, "MintFreshSmall", 100_000_000)
	f.pricePoint(t, "MintFreshSmall", 100_000_000, time.Minute)

	got, err := treasury.New(module.Deps{Pool: f.pool}).
		TopHoldings().
		TopHoldings(t.Context(), []ids.CabalID{risen, stale})
	want := map[ids.CabalID]chain.SolanaAddress{risen: "MintTripled", stale: "MintStaleBig"}
	if err != nil || len(got) != len(want) || got[risen] != want[risen] || got[stale] != want[stale] {
		t.Fatalf(
			"TopHoldings = %v, %v, want %v: the holding that tripled beats the bigger buy that fell, and a stale price uses cost basis",
			got,
			err,
			want,
		)
	}
}
