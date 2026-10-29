package treasury_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestTopHoldings_picksEachCabalsLargestCostBasisAmongHeldAssets(t *testing.T) {
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
