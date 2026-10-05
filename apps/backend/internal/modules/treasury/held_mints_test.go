package treasury_test

import (
	"context"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestHeldMints_namesEveryMintACabalHoldsOnce(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	first, second := f.cabal(t), f.cabal(t)
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, $2, $3, 0, $4)`
	for _, row := range []struct {
		cabal any
		mint  string
		units int64
	}{
		{first.UUID(), "MintHeld", 5},
		{second.UUID(), "MintHeld", 7},
		{first.UUID(), "MintSold", 0},
		{second.UUID(), "MintOther", 1},
	} {
		if _, err := f.pool.Exec(t.Context(), insert, row.cabal, row.mint, row.units, f.clock.Now()); err != nil {
			t.Fatal(err)
		}
	}
	held := treasury.New(module.Deps{Pool: f.pool})
	got, err := held.HeldMints(t.Context())
	if err != nil || !slices.Equal(got, []chain.SolanaAddress{"MintHeld", "MintOther"}) {
		t.Fatalf("HeldMints = %v, %v, want MintHeld and MintOther, not the sold-out MintSold", got, err)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := held.HeldMints(ctx); err == nil {
		t.Fatal("HeldMints on a cancelled context succeeded")
	}
}
