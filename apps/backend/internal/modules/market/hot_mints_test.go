package market_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestModule_wiresHeldAndProposedMintsIntoThePricePoller(t *testing.T) {
	t.Parallel()
	pool, g := testkit.DB(t), testkit.NewIDs(3)
	now := clock.Real{}.Now()
	const hold = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, 'MintHeld', 3, 0, $2)`
	if _, err := pool.Exec(t.Context(), hold, g.NewV7(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO proposals (id, cabal_id, proposer_id, kind, symbol, mint,
  usdc_micros, quote_out_amount, threshold, status, expires_at, created_at, updated_at)
VALUES ($1, $2, $3, 'buy', 'PRPx', 'MintProposed', 5, 1, 'majority', 'open', $4, $4, $4)`,
		g.NewV7(), g.NewV7(), g.NewV7(), now); err != nil {
		t.Fatal(err)
	}
	d := module.Deps{Pool: pool, Config: moduleConfig(), Clock: clock.Real{}}
	m := market.New(d)
	module.NewSet(m, treasury.New(d), governance.New(d))

	var got []chain.SolanaAddress
	for _, read := range market.HotMints(m) {
		mints, err := read(t.Context())
		if err != nil {
			t.Fatal(err)
		}
		got = append(got, mints...)
	}
	slices.Sort(got)
	if !slices.Equal(got, []chain.SolanaAddress{"MintHeld", "MintProposed"}) {
		t.Fatalf("the price poller's hot mints = %v, want MintHeld and MintProposed", got)
	}
}
