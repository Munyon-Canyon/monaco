package adapters

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type heldStub struct {
	rows []sqlc.HeldByCabalRow
	err  error
}

func (s heldStub) HeldByCabal(context.Context) ([]sqlc.HeldByCabalRow, error) { return s.rows, s.err }

type activeStub struct {
	active map[ids.CabalID]bool
	calls  [][]ids.CabalID
	err    error
}

func (s *activeStub) Active(_ context.Context, in []ids.CabalID) (map[ids.CabalID]bool, error) {
	s.calls = append(s.calls, in)
	return s.active, s.err
}

func holderRows(cabal uuid.UUID, mints ...string) []sqlc.HeldByCabalRow {
	rows := make([]sqlc.HeldByCabalRow, len(mints))
	for i, mint := range mints {
		rows[i] = sqlc.HeldByCabalRow{Asset: mint, CabalID: cabal}
	}
	return rows
}

func holders(rows []sqlc.HeldByCabalRow, active *activeStub) CabalHolders {
	return CabalHolders{held: heldStub{rows: rows}, active: active.Active}
}

func TestCabalHolders_countsActiveCabalsPerMintWithOneCabalCall(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(4)
	a, b, c, banned := g.NewV7(), g.NewV7(), g.NewV7(), g.NewV7()
	rows := slices.Concat(
		holderRows(a, "NVDA", "AAPL"), holderRows(b, "NVDA"), holderRows(c, "NVDA", "AAPL"),
		holderRows(banned, "NVDA", "AAPL", "TSLA"),
	)
	active := &activeStub{active: map[ids.CabalID]bool{
		ids.CabalIDFrom(a): true, ids.CabalIDFrom(b): true, ids.CabalIDFrom(c): true, ids.CabalIDFrom(banned): false,
	}}
	got, err := holders(rows, active).CabalHolderCounts(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	want := []port.HolderCount{{Mint: "AAPL", Cabals: 2}, {Mint: "NVDA", Cabals: 3}}
	if !slices.Equal(got, want) {
		t.Fatalf("counts = %+v, want %+v (a banned cabal must not count, and TSLA has no active holder)", got, want)
	}
	if len(active.calls) != 1 || len(active.calls[0]) != 4 {
		t.Fatalf("cabal port calls = %v, want one call with the four distinct cabals", active.calls)
	}
}

func TestCabalHolders_neverCountsACabalTheCabalPortDoesNotKnow(t *testing.T) {
	t.Parallel()
	cabal := testkit.NewIDs(1).NewV7()
	got, err := holders(holderRows(cabal, "NVDA"), &activeStub{}).CabalHolderCounts(t.Context())
	if err != nil || len(got) != 0 {
		t.Fatalf("counts = %+v, %v, want none", got, err)
	}
}

func TestCabalHolders_asksNothingWhenNoCabalHoldsAnything(t *testing.T) {
	t.Parallel()
	active := &activeStub{}
	got, err := holders(nil, active).CabalHolderCounts(t.Context())
	if err != nil || len(got) != 0 || len(active.calls) != 0 {
		t.Fatalf("counts = %+v, %v, calls = %v, want none", got, err, active.calls)
	}
}

func TestCabalHolders_reportsEachReadFailure(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "treasury.test")
	rows := holderRows(testkit.NewIDs(1).NewV7(), "NVDA")
	for name, h := range map[string]CabalHolders{
		"positions": {held: heldStub{err: boom}, active: (&activeStub{}).Active},
		"cabals":    holders(rows, &activeStub{err: boom}),
	} {
		if _, err := h.CabalHolderCounts(t.Context()); errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("%s: err = %v, want internal", name, err)
		}
	}
}
