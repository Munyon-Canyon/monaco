package treasury_test

import (
	"context"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
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

type cabalStatuses struct {
	cabalport.Queries
	status map[ids.CabalID]cabalport.Status
	calls  int
	err    error
}

func (c *cabalStatuses) Cabals(_ context.Context, in []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error) {
	c.calls++
	if c.err != nil {
		return nil, c.err
	}
	out := map[ids.CabalID]cabalport.CabalView{}
	for _, id := range in {
		out[id] = cabalport.CabalView{ID: id, Status: c.status[id]}
	}
	return out, nil
}

type cabalModule struct {
	module.Module
	queries *cabalStatuses
}

func (cabalModule) Name() string { return "cabalstub" }

func (c cabalModule) Queries() cabalport.Queries { return c.queries }

func TestCabalHolderCounts_countsOnlyActiveCabalsHoldingUnits(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	first, second, banned := f.cabal(t), f.cabal(t), f.cabal(t)
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, $2, $3, 0, $4)`
	for _, row := range []struct {
		cabal ids.CabalID
		mint  string
		units int64
	}{
		{first, "MintHeld", 5},
		{second, "MintHeld", 7},
		{banned, "MintHeld", 9},
		{first, "MintSold", 0},
		{banned, "MintBannedOnly", 3},
	} {
		if _, err := f.pool.Exec(
			t.Context(), insert, row.cabal.UUID(), row.mint, row.units, f.clock.Now(),
		); err != nil {
			t.Fatal(err)
		}
	}
	cabals := &cabalStatuses{status: map[ids.CabalID]cabalport.Status{
		first: cabalport.StatusActive, second: cabalport.StatusActive, banned: cabalport.StatusBanned,
	}}
	held := treasury.New(module.Deps{Pool: f.pool})
	module.NewSet(held, cabalModule{queries: cabals})
	got, err := held.CabalHolderCounts(t.Context())
	want := []port.HolderCount{{Mint: "MintHeld", Cabals: 2}}
	if err != nil || !slices.Equal(got, want) {
		t.Fatalf("CabalHolderCounts = %+v, %v, want %+v (no zero-unit MintSold, no banned-only mint)", got, err, want)
	}
	if cabals.calls != 1 {
		t.Fatalf("cabal port calls = %d, want 1 for three cabals", cabals.calls)
	}
}

func TestCabalHolderCounts_reportsAFailingCabalPort(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	const insert = `INSERT INTO cabal_positions (cabal_id, asset, units, cost_basis_micros, updated_at)
VALUES ($1, 'MintHeld', 5, 0, $2)`
	if _, err := f.pool.Exec(t.Context(), insert, f.cabal(t).UUID(), f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	held := treasury.New(module.Deps{Pool: f.pool})
	module.NewSet(held, cabalModule{queries: &cabalStatuses{err: errs.New(errs.CodeInternal, "treasury.test")}})
	if _, err := held.CabalHolderCounts(t.Context()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("CabalHolderCounts error = %v, want internal", err)
	}
}
