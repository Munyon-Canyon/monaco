package market_test

import (
	"slices"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/governance"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
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

func TestModule_retentionKeepsHistoryOfHeldAndProposedListedMints(t *testing.T) {
	t.Parallel()
	pool, g := testkit.DB(t), testkit.NewIDs(3)
	now := retentionNow()
	clk := testkit.NewClock(now)
	held, proposed, cold := marketfake.TSpaceX(), marketfake.SPACEX(), marketfake.TSLAx()
	cold.PopularRank = 0
	seedAssets(t, pool, now, held, proposed, cold)
	const hold = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, $2, 3, 0, $3)`
	if _, err := pool.Exec(t.Context(), hold, g.NewV7(), held.Mint.String(), now); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO proposals (id, cabal_id, proposer_id, kind, symbol, mint,
  usdc_micros, quote_out_amount, threshold, status, expires_at, created_at, updated_at)
VALUES ($1, $2, $3, 'buy', 'SPACEX', $4, 5, 1, 'majority', 'open', $5, $5, $5)`,
		g.NewV7(), g.NewV7(), g.NewV7(), proposed.Mint.String(), now); err != nil {
		t.Fatal(err)
	}
	old := now.Add(-30 * 24 * time.Hour)
	insertPrices(t, pool, []priceRow{
		{mint: held.Mint.String(), ts: old, micros: 100_000_000, source: "jupiter"},
		{mint: proposed.Mint.String(), ts: old, micros: 100_000_000, source: "jupiter"},
		{mint: cold.Mint.String(), ts: old, micros: 100_000_000, source: "jupiter"},
	})
	d := module.Deps{Pool: pool, Config: moduleConfig(), Clock: clk, IDs: g, UoW: db.New(pool, g, clk)}
	m := market.New(d)
	module.NewSet(m, treasury.New(d), governance.New(d))
	if _, err := m.Retention().Tick(t.Context()); err != nil {
		t.Fatal(err)
	}
	rows := loadPrices(t, pool)
	got := make([]string, 0, len(rows))
	for _, row := range rows {
		got = append(got, row.mint)
	}
	slices.Sort(got)
	if want := []string{
		held.Mint.String(),
		proposed.Mint.String(),
	}; !slices.Equal(
		got,
		slices.Sorted(slices.Values(want)),
	) {
		t.Fatalf("mints with history after retention = %v, want the held and proposed listed mints %v", got, want)
	}
}
