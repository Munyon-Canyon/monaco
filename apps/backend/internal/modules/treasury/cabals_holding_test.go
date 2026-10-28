package treasury_test

import (
	"testing"

	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func TestCabalsHolding_countsActiveCabalsHoldingOneAsset(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	first, banned := f.cabal(t), f.cabal(t)
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, 'MintHeld', 5, 0, $2)`
	for _, id := range []ids.CabalID{first, banned} {
		if _, err := f.pool.Exec(t.Context(), insert, id.UUID(), f.clock.Now()); err != nil {
			t.Fatal(err)
		}
	}
	cabals := &cabalStatuses{status: map[ids.CabalID]cabalport.Status{
		first: cabalport.StatusActive, banned: cabalport.StatusBanned,
	}}
	held := treasury.New(module.Deps{Pool: f.pool})
	module.NewSet(held, cabalModule{queries: cabals})
	if got, err := held.CabalsHolding(t.Context(), "MintHeld"); err != nil || got != 1 {
		t.Fatalf("CabalsHolding(MintHeld) = %d, %v, want 1 (the banned cabal does not count)", got, err)
	}
	if got, err := held.CabalsHolding(t.Context(), "MintNone"); err != nil || got != 0 {
		t.Fatalf("CabalsHolding(MintNone) = %d, %v, want 0", got, err)
	}
}
