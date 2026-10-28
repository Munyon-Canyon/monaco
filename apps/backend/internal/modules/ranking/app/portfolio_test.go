package app_test

import (
	"context"
	"math"
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

type fakeOwnership struct {
	bps map[ids.CabalID]int32
	err error
}

func (f fakeOwnership) OwnershipBps(context.Context, ids.UserID) (map[ids.CabalID]int32, error) {
	return f.bps, f.err
}

type fakeRoles struct {
	members []cabal.MemberView
	err     error
}

func (f fakeRoles) MembersOf(_ context.Context, cabals []ids.CabalID) (map[ids.CabalID][]cabal.MemberView, error) {
	out := map[ids.CabalID][]cabal.MemberView{}
	for _, id := range cabals {
		out[id] = f.members
	}
	return out, f.err
}

type portfolioFixture struct {
	user      ids.UserID
	run       domain.Run
	cabal     ids.CabalID
	stakes    fakeStakes
	latest    *fakeLatest
	cards     *fakeCards
	ownership fakeOwnership
	roles     fakeRoles
}

func newPortfolioFixture() portfolioFixture {
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	cabalID := ids.CabalIDFrom(ids.Real{}.NewV7())
	user := ids.UserIDFrom(ids.Real{}.NewV7())
	return portfolioFixture{
		user:  user,
		run:   domain.Run{ID: ids.Real{}.NewV7(), FinishedAt: at.Add(time.Second), PricesAsOf: at.Add(-time.Minute)},
		cabal: cabalID,
		stakes: fakeStakes{points: []app.StakePoint{{
			CabalID: cabalID, At: at.Add(-time.Hour), ShareUnits: money.SharesUnitsFromUint64(5),
			NetContributed: money.SignedMicrosFromInt64(100),
		}}},
		latest: &fakeLatest{values: map[ids.CabalID]domain.Snapshot{
			cabalID: {At: at, Value: money.MicrosFromUint64(400), TotalShares: money.SharesUnitsFromUint64(10)},
		}},
		cards:     &fakeCards{cards: map[ids.CabalID]cabal.View{cabalID: {ID: cabalID, Name: "Alpha"}}},
		ownership: fakeOwnership{bps: map[ids.CabalID]int32{cabalID: 5000}},
		roles:     fakeRoles{members: []cabal.MemberView{{UserID: user, Role: cabal.RoleMember, CanVote: true}}},
	}
}

func (f portfolioFixture) read(t *testing.T, boards fakeBoards) (app.PortfolioView, error) {
	t.Helper()
	return app.ReadPortfolio{User: f.user}.Run(t.Context(), app.PortfolioPorts{
		Boards: boards, Latest: f.latest, Stakes: f.stakes, Cards: f.cards, Ownership: f.ownership, Roles: f.roles,
	})
}

func TestReadPortfolio_TakesOwnershipFromTreasuryUnchanged(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	f.ownership.bps[f.cabal] = 3334
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 1 || got.Rows[0].OwnershipBps != 3334 || got.Rows[0].SliceBps != 10_000 {
		t.Fatalf("Run = %+v, %v, want ownership 3334 from treasury and slice 10000", got, err)
	}
}

func TestReadPortfolio_ReadsZeroOwnershipForAStakeTreasuryNoLongerHolds(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	f.ownership.bps = map[ids.CabalID]int32{}
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 1 || got.Rows[0].OwnershipBps != 0 || got.Rows[0].Value.Uint64() != 200 {
		t.Fatalf("Run = %+v, %v, want the row at the snapshot with ownership 0", got, err)
	}
}

func TestReadPortfolio_MapsTheCallersRole(t *testing.T) {
	t.Parallel()
	other := cabal.MemberView{UserID: ids.UserIDFrom(ids.Real{}.NewV7()), Role: cabal.RoleCreator, CanVote: true}
	for name, tc := range map[string]struct {
		me   *cabal.MemberView
		want app.Membership
	}{
		"creator":    {me: &cabal.MemberView{Role: cabal.RoleCreator, CanVote: true}, want: app.Membership{Role: app.RoleAdmin, CanVote: true}},
		"voter":      {me: &cabal.MemberView{Role: cabal.RoleMember, CanVote: true}, want: app.Membership{Role: app.RoleMember, CanVote: true}},
		"non-voter":  {me: &cabal.MemberView{Role: cabal.RoleMember}, want: app.Membership{Role: app.RoleMember}},
		"left since": {want: app.Membership{Role: app.RoleMember}},
	} {
		f := newPortfolioFixture()
		f.roles.members = []cabal.MemberView{other}
		if tc.me != nil {
			tc.me.UserID = f.user
			f.roles.members = append(f.roles.members, *tc.me)
		}
		got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
		if err != nil || got.Members[f.cabal] != tc.want {
			t.Errorf("%s: Members = %+v, %v, want %+v", name, got.Members, err, tc.want)
		}
	}
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

func TestReadPortfolio_SkipsACabalWhoseStakeOutgrowsItsSnapshotInsteadOfFailing(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	f.latest.values[f.cabal] = domain.Snapshot{
		At:          f.run.PricesAsOf.Add(time.Hour),
		Value:       money.MicrosFromUint64(400),
		TotalShares: money.SharesUnitsFromUint64(2),
	}
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 0 || got.Total.Uint64() != 0 || f.cards.queries != 0 {
		t.Fatalf("oversold cabal = %+v, %v, want an empty portfolio and no error", got, err)
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
			f.latest.values[f.cabal] = domain.Snapshot{
				At: f.run.PricesAsOf.Add(time.Hour), Value: money.MicrosFromUint64(math.MaxUint64),
				TotalShares: money.SharesUnitsFromUint64(5),
			}
			return good
		}},
		"ownership":    {func(f *portfolioFixture) fakeBoards { f.ownership.err = boom; return good }},
		"role":         {func(f *portfolioFixture) fakeBoards { f.roles.err = boom; return good }},
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
