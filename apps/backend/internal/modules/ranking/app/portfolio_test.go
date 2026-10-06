package app_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type fakeLatest struct {
	values  map[ids.CabalID]domain.Snapshot
	err     error
	queries int
}

func (f *fakeLatest) LatestValuesOf(context.Context, []ids.CabalID) (map[ids.CabalID]domain.Snapshot, error) {
	f.queries++
	return f.values, f.err
}

type fakeCards struct {
	cards   map[ids.CabalID]cabal.View
	err     error
	queries int
}

func (f *fakeCards) Cabals(context.Context, []ids.CabalID) (map[ids.CabalID]cabal.View, error) {
	f.queries++
	return f.cards, f.err
}

type portfolioFixture struct {
	run    domain.Run
	cabal  ids.CabalID
	stakes fakeStakes
	latest *fakeLatest
	cards  *fakeCards
}

func newPortfolioFixture() portfolioFixture {
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	return portfolioFixture{
		run:   domain.Run{ID: ids.Real{}.NewV7(), FinishedAt: at.Add(time.Second), PricesAsOf: at.Add(-time.Minute)},
		cabal: cabalID,
		stakes: fakeStakes{points: []app.StakePoint{{
			CabalID: cabalID, At: at.Add(-time.Hour), ShareUnits: money.SharesUnitsFromUint64(5),
			NetContributed: money.SignedMicrosFromInt64(100),
		}}},
		latest: &fakeLatest{values: map[ids.CabalID]domain.Snapshot{
			cabalID: {At: at, Value: money.MicrosFromUint64(400), TotalShares: money.SharesUnitsFromUint64(10)},
		}},
		cards: &fakeCards{cards: map[ids.CabalID]cabal.View{cabalID: {ID: cabalID, Name: "Alpha"}}},
	}
}

func (f portfolioFixture) read(t *testing.T, boards fakeBoards) (app.PortfolioView, error) {
	t.Helper()
	return app.ReadPortfolio{
		User: ids.UserIDFrom(ids.Real{}.NewV7()),
	}.Run(
		t.Context(),
		boards,
		f.latest,
		f.stakes,
		f.cards,
	)
}

func TestReadPortfolio_ValuesTheStakeAndNamesTheCabal(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 1 || got.Rows[0].Value.Uint64() != 200 || got.Rows[0].SliceBps != 10_000 ||
		got.Total.Uint64() != 200 || got.PnL.Int64() != 100 || got.Cabals[f.cabal].Name != "Alpha" ||
		!got.ComputedAt.Equal(f.run.FinishedAt) || !got.PricesAsOf.Equal(f.run.PricesAsOf) {
		t.Fatalf("Run = %+v, %v", got, err)
	}
}

func TestReadPortfolio_IsEmptyWithoutARun(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	got, err := f.read(t, fakeBoards{})
	if err != nil || got.Rows == nil || len(got.Rows) != 0 || got.ComputedAt != nil || got.PricesAsOf != nil {
		t.Fatalf("no run = %+v, %v, want empty rows and null times", got, err)
	}
}

func TestReadPortfolio_IsEmptyWithoutAStakeAndReadsNoSnapshots(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	f.stakes = fakeStakes{}
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 0 || got.ComputedAt == nil || f.latest.queries != 0 {
		t.Fatalf("no stake = %+v, %v, want empty rows with the run's times and no snapshot read", got, err)
	}
}

func TestReadPortfolio_IsEmptyWithoutAValuedCabalAndReadsNoNames(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	f.latest = &fakeLatest{}
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 0 || f.cards.queries != 0 {
		t.Fatalf("no snapshot = %+v, %v, want empty rows and no name read", got, err)
	}
}

func TestReadPortfolio_Failures(t *testing.T) {
	t.Parallel()
	boom := errs.New(errs.CodeInternal, "test")
	good := fakeBoards{}
	for name, tc := range map[string]struct {
		change func(*portfolioFixture) fakeBoards
	}{
		"latest run": {func(*portfolioFixture) fakeBoards { return fakeBoards{failing: "run"} }},
		"stakes":     {func(f *portfolioFixture) fakeBoards { f.stakes = fakeStakes{err: boom}; return good }},
		"snapshots":  {func(f *portfolioFixture) fakeBoards { f.latest.err = boom; return good }},
		"portfolio": {func(f *portfolioFixture) fakeBoards {
			f.latest.values[f.cabal] = domain.Snapshot{At: f.run.PricesAsOf.Add(time.Hour), TotalShares: money.SharesUnitsFromUint64(1)}
			return good
		}},
		"names":        {func(f *portfolioFixture) fakeBoards { f.cards.err = boom; return good }},
		"missing name": {func(f *portfolioFixture) fakeBoards { f.cards.cards = nil; return good }},
	} {
		f := newPortfolioFixture()
		boards := tc.change(&f)
		boards.run, boards.hasRun = f.run, boards.failing == ""
		if _, err := f.read(t, boards); errs.CodeOf(err) != errs.CodeInternal {
			t.Errorf("%s: err = %v, want internal", name, err)
		}
	}
}
