package app_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	fundingport "github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
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

type fakeBalances struct {
	available money.Micros
	inFlight  money.Micros
	err       error
	queries   int
}

func (f *fakeBalances) ForDisplay(context.Context, ids.UserID) (fundingport.Balance, error) {
	f.queries++
	return fundingport.Balance{AvailableMicros: f.available, InFlightFundMicros: f.inFlight}, f.err
}

type fakeJoined struct {
	cabals  []ids.CabalID
	err     error
	queries int
}

func (f *fakeJoined) CabalsOf(context.Context, ids.UserID) ([]ids.CabalID, error) {
	f.queries++
	return f.cabals, f.err
}

type fakeVotes struct {
	counts  map[ids.CabalID]int
	err     error
	queries int
}

func (f *fakeVotes) OpenVoteCounts(context.Context, []ids.CabalID) (map[ids.CabalID]int, error) {
	f.queries++
	return f.counts, f.err
}

type portfolioFixture struct {
	balances  *fakeBalances
	joined    *fakeJoined
	votes     *fakeVotes
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
		user:     user,
		balances: &fakeBalances{available: money.MicrosFromUint64(50)},
		joined:   &fakeJoined{cabals: []ids.CabalID{cabalID}},
		votes:    &fakeVotes{counts: map[ids.CabalID]int{}},
		run:      domain.Run{ID: ids.Real{}.NewV7(), FinishedAt: at.Add(time.Second), PricesAsOf: at.Add(-time.Minute)},
		cabal:    cabalID,
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
		Balances:  f.balances,
		Joined:    f.joined,
		Votes:     f.votes,
		Boards:    boards,
		Latest:    f.latest,
		Stakes:    f.stakes,
		Cards:     f.cards,
		Ownership: f.ownership,
		Roles:     f.roles,
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
		"creator": {me: &cabal.MemberView{Role: cabal.RoleCreator, CanVote: true}, want: app.Membership{Role: app.RoleAdmin, CanVote: true}},
		"admin": {
			me:   &cabal.MemberView{Role: cabal.Role("admin"), CanVote: true},
			want: app.Membership{Role: app.RoleAdmin, CanVote: true},
		},
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

func TestReadPortfolio_ListsACabalWhoseStakeOutgrowsItsSnapshotAsAZeroRowInsteadOfFailing(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	f.latest.values[f.cabal] = domain.Snapshot{
		At:          f.run.PricesAsOf.Add(time.Hour),
		Value:       money.MicrosFromUint64(400),
		TotalShares: money.SharesUnitsFromUint64(2),
	}
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 1 || got.Rows[0].Value.Uint64() != 0 || got.Total.Uint64() != 0 ||
		got.NetWorth.Uint64() != 50 {
		t.Fatalf("oversold cabal = %+v, %v, want a zero row and no error", got, err)
	}
}

func TestReadPortfolio_IsEmptyWithoutARun(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	f.joined.cabals = nil
	got, err := f.read(t, fakeBoards{})
	if err != nil || got.Rows == nil || len(got.Rows) != 0 || got.ComputedAt != nil || got.PricesAsOf != nil ||
		got.Cash.Uint64() != 50 || got.NetWorth.Uint64() != 50 || f.votes.queries != 0 {
		t.Fatalf("no run = %+v, %v, want cash as net worth, empty rows and null times", got, err)
	}
}

func TestReadPortfolio_IsEmptyWithoutAStakeAndReadsNoSnapshots(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	f.joined.cabals = nil
	f.stakes = fakeStakes{}
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 0 || got.ComputedAt == nil || f.latest.queries != 0 {
		t.Fatalf("no stake = %+v, %v, want empty rows with the run's times and no snapshot read", got, err)
	}
}

func TestReadPortfolio_ListsAMembershipWithoutASnapshotAsAZeroRow(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	f.latest = &fakeLatest{}
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 1 || got.Rows[0].Value.Uint64() != 0 || got.Rows[0].PotValue.Uint64() != 0 ||
		got.Cabals[f.cabal].Name != "Alpha" {
		t.Fatalf("no snapshot = %+v, %v, want the membership as a zero row", got, err)
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
		"net worth": {func(f *portfolioFixture) fakeBoards {
			f.balances.available = money.MicrosFromUint64(math.MaxUint64)
			return good
		}},
		"cash":         {func(f *portfolioFixture) fakeBoards { f.balances.err = boom; return good }},
		"memberships":  {func(f *portfolioFixture) fakeBoards { f.joined.err = boom; return good }},
		"open votes":   {func(f *portfolioFixture) fakeBoards { f.votes.err = boom; return good }},
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

type multiFixture struct {
	portfolioFixture
	cabals []ids.CabalID
}

func newMultiFixture() multiFixture {
	f := newPortfolioFixture()
	at := f.run.PricesAsOf.Add(time.Minute)
	cabals := make([]ids.CabalID, 1, 4)
	cabals[0] = f.cabal
	for range 3 {
		cabals = append(cabals, ids.CabalIDFrom(ids.Real{}.NewV7()))
	}
	pots := []uint64{20_000_000_000, 22_580_000_000, 23_400_000_000}
	counts := []int{8, 6, 6, 3}
	shares := []uint64{35, 12, 8}
	f.stakes = fakeStakes{}
	f.latest = &fakeLatest{values: map[ids.CabalID]domain.Snapshot{}}
	f.cards = &fakeCards{cards: map[ids.CabalID]cabal.View{}}
	f.ownership = fakeOwnership{bps: map[ids.CabalID]int32{}}
	for i, id := range cabals {
		f.cards.cards[id] = cabal.View{ID: id, Name: "Cabal", MemberCount: counts[i]}
		if i == 3 {
			continue
		}
		f.stakes.points = append(f.stakes.points, app.StakePoint{
			CabalID: id, At: at.Add(-time.Hour), ShareUnits: money.SharesUnitsFromUint64(shares[i]),
			NetContributed: money.SignedMicrosFromInt64(1),
		})
		f.latest.values[id] = domain.Snapshot{
			At:          at,
			Value:       money.MicrosFromUint64(pots[i]),
			TotalShares: money.SharesUnitsFromUint64(100),
		}
		f.ownership.bps[id] = int32(i)
	}
	f.joined.cabals = cabals
	f.votes.counts = map[ids.CabalID]int{cabals[0]: 1}
	f.balances.available = money.MicrosFromUint64(500_000_000)
	return multiFixture{portfolioFixture: f, cabals: cabals}
}

func TestReadPortfolio_ServesNetWorthTreasuriesMemberCountsAndOpenVotesForEveryMembership(t *testing.T) {
	t.Parallel()
	f := newMultiFixture()
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 4 {
		t.Fatalf("Run = %+v, %v, want four rows", got, err)
	}
	if got.Cash.Uint64() != 500_000_000 || got.NetWorth.Uint64() != got.Total.Uint64()+500_000_000 {
		t.Fatalf("cash %v net worth %v total %v, want net worth = total + cash", got.Cash, got.NetWorth, got.Total)
	}
	wantPot := map[ids.CabalID]uint64{
		f.cabals[0]: 20_000_000_000, f.cabals[1]: 22_580_000_000, f.cabals[2]: 23_400_000_000, f.cabals[3]: 0,
	}
	wantMembers := map[ids.CabalID]int{f.cabals[0]: 8, f.cabals[1]: 6, f.cabals[2]: 6, f.cabals[3]: 3}
	for _, row := range got.Rows {
		if row.PotValue.Uint64() != wantPot[row.CabalID] ||
			got.Cabals[row.CabalID].MemberCount != wantMembers[row.CabalID] {
			t.Errorf("cabal %v pot %v members %d, want %d and %d", row.CabalID, row.PotValue,
				got.Cabals[row.CabalID].MemberCount, wantPot[row.CabalID], wantMembers[row.CabalID])
		}
	}
	if got.OpenVotes[f.cabals[0]] != 1 || got.OpenVotes[f.cabals[1]] != 0 || got.OpenVotes[f.cabals[3]] != 0 {
		t.Errorf("open votes = %v, want 1 for the first cabal only", got.OpenVotes)
	}
}

func TestReadPortfolio_ListsTheUnfundedMembershipLastWithZeroValueAndOwnership(t *testing.T) {
	t.Parallel()
	f := newMultiFixture()
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 4 {
		t.Fatalf("Run = %+v, %v, want four rows", got, err)
	}
	if last := got.Rows[3]; last.CabalID != f.cabals[3] || !last.Value.IsZero() || last.OwnershipBps != 0 {
		t.Errorf("unfunded row = %+v, want last, zero and unowned", last)
	}
}

func TestReadPortfolio_MakesOneCallToEachPortForManyCabals(t *testing.T) {
	t.Parallel()
	f := newMultiFixture()
	if _, err := f.read(t, fakeBoards{run: f.run, hasRun: true}); err != nil {
		t.Fatalf("Run: %v", err)
	}
	if f.balances.queries != 1 || f.joined.queries != 1 || f.votes.queries != 1 || f.latest.queries != 1 ||
		f.cards.queries != 1 {
		t.Fatalf("calls: balances %d, memberships %d, votes %d, snapshots %d, cards %d, want one each",
			f.balances.queries, f.joined.queries, f.votes.queries, f.latest.queries, f.cards.queries)
	}
}

func TestReadPortfolio_ACashOnlyUserGetsTheirCashAsNetWorth(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	f.joined.cabals, f.stakes = nil, fakeStakes{}
	got, err := f.read(t, fakeBoards{})
	if err != nil || got.Rows == nil || len(got.Rows) != 0 || got.NetWorth.Uint64() != 50 || got.Cash.Uint64() != 50 {
		t.Fatalf("Run = %+v, %v, want cash and net worth with an empty list", got, err)
	}
}

func TestReadPortfolio_AUserWhoseOnlyCabalHasNoValuationRunYetGetsAZeroRow(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	got, err := f.read(t, fakeBoards{})
	if err != nil || len(got.Rows) != 1 || got.Rows[0].CabalID != f.cabal || !got.Rows[0].Value.IsZero() ||
		got.Cabals[f.cabal].Name != "Alpha" || got.ComputedAt != nil || f.latest.queries != 0 {
		t.Fatalf("Run = %+v, %v, want the membership as a zero row and no snapshot read", got, err)
	}
}

func TestReadPortfolio_FundingNeverLowersNetWorth(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	netWorth := func(t *testing.T, mutate func(*portfolioFixture)) app.PortfolioView {
		t.Helper()
		f := newPortfolioFixture()
		f.balances.available = money.MicrosFromUint64(100)
		mutate(&f)
		got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
		if err != nil {
			t.Fatal(err)
		}
		return got
	}
	before := netWorth(t, func(*portfolioFixture) {})
	if before.NetWorth.Uint64() != 300 || before.Arriving.Int64() != 0 {
		t.Fatalf(
			"before = %d net worth, %d arriving, want 300 and 0",
			before.NetWorth.Uint64(),
			before.Arriving.Int64(),
		)
	}
	cases := map[string]func(*portfolioFixture){
		"a fund in flight": func(f *portfolioFixture) {
			f.balances.available, f.balances.inFlight = money.MicrosFromUint64(70), money.MicrosFromUint64(30)
		},
		"a fund settled since the run": func(f *portfolioFixture) {
			f.balances.available = money.MicrosFromUint64(70)
			f.stakes.points = append(f.stakes.points, app.StakePoint{
				CabalID: f.cabal, At: at.Add(time.Minute), ShareUnits: money.SharesUnitsFromUint64(8),
				NetContributed: money.SignedMicrosFromInt64(130),
			})
		},
	}
	for name, mutate := range cases {
		got := netWorth(t, mutate)
		if got.NetWorth.Uint64() != 300 || got.Arriving.Int64() != 30 {
			t.Errorf("%s: net worth %d arriving %d, want 300 and 30", name, got.NetWorth.Uint64(), got.Arriving.Int64())
		}
	}
}

func TestReadPortfolio_ACashOutSettledSinceTheRunDoesNotCountTwice(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	f := newPortfolioFixture()
	f.balances.available = money.MicrosFromUint64(150)
	f.stakes.points = append(f.stakes.points, app.StakePoint{
		CabalID: f.cabal, At: at.Add(time.Minute), ShareUnits: money.SharesUnitsFromUint64(3),
		NetContributed: money.SignedMicrosFromInt64(0),
	})
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || got.NetWorth.Uint64() != 250 || got.Arriving.Int64() != -100 {
		t.Fatalf(
			"Run = net worth %d arriving %d, %v, want 250 and -100",
			got.NetWorth.Uint64(),
			got.Arriving.Int64(),
			err,
		)
	}
}

func TestReadPortfolio_FlagsACabalTheLatestRunSkipped(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	f.run.AsOf = f.latest.values[f.cabal].At.Add(time.Hour)
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 1 || !got.Rows[0].Stale {
		t.Fatalf("Run = %+v, %v, want the row flagged stale", got.Rows, err)
	}
	f.run.AsOf = f.latest.values[f.cabal].At
	if got, err = f.read(t, fakeBoards{run: f.run, hasRun: true}); err != nil || got.Rows[0].Stale {
		t.Fatalf("Run = %+v, %v, want a cabal in the latest run left unflagged", got.Rows, err)
	}
}

func TestReadPortfolio_ASkippedCabalCarriesTheTimeOfItsLastValuation(t *testing.T) {
	t.Parallel()
	f := newPortfolioFixture()
	valued := f.latest.values[f.cabal].At
	f.run.AsOf = valued.Add(time.Hour)
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || len(got.Rows) != 1 || got.Rows[0].ValuedAt == nil || !got.Rows[0].ValuedAt.Equal(valued) {
		t.Fatalf("Run = %+v, %v, want the stale row to carry %v", got.Rows, err, valued)
	}
	f.run.AsOf = valued
	if got, err = f.read(t, fakeBoards{run: f.run, hasRun: true}); err != nil || got.Rows[0].ValuedAt != nil {
		t.Fatalf("Run = %+v, %v, want no valuation time on a cabal in the latest run", got.Rows, err)
	}
	f.latest.values = map[ids.CabalID]domain.Snapshot{}
	f.run.AsOf = valued.Add(time.Hour)
	if got, err = f.read(t, fakeBoards{run: f.run, hasRun: true}); err != nil || got.Rows[0].ValuedAt != nil {
		t.Fatalf("Run = %+v, %v, want no valuation time on a cabal with no snapshot", got.Rows, err)
	}
}

func TestReadPortfolio_RefusesMoneyThatCannotBeAddedUp(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	for name, mutate := range map[string]func(*portfolioFixture){
		"a fund in flight larger than any balance": func(f *portfolioFixture) {
			f.balances.inFlight = money.MicrosFromUint64(math.MaxUint64)
		},
		"money arriving on top of a fund in flight": func(f *portfolioFixture) {
			f.balances.inFlight = money.MicrosFromUint64(math.MaxInt64)
			f.stakes.points = append(f.stakes.points, app.StakePoint{
				CabalID: f.cabal, At: at.Add(time.Minute), ShareUnits: money.SharesUnitsFromUint64(8),
				NetContributed: money.SignedMicrosFromInt64(130),
			})
		},
		"cash that overflows the cabals' total": func(f *portfolioFixture) {
			f.balances.available = money.MicrosFromUint64(math.MaxUint64)
		},
		"arriving money that overflows net worth": func(f *portfolioFixture) {
			f.balances.available = money.MicrosFromUint64(math.MaxUint64 - 250)
			f.balances.inFlight = money.MicrosFromUint64(100)
		},
	} {
		f := newPortfolioFixture()
		mutate(&f)
		if _, err := f.read(t, fakeBoards{run: f.run, hasRun: true}); errs.CodeOf(err) == "" {
			t.Errorf("%s: err = %v, want a refusal", name, err)
		}
	}
}

func TestReadPortfolio_ACashOutLargerThanEverythingElseShowsZeroNotADebt(t *testing.T) {
	t.Parallel()
	at := time.Date(2026, 10, 4, 12, 0, 0, 0, time.UTC)
	f := newPortfolioFixture()
	f.balances.available = money.MicrosFromUint64(0)
	f.stakes.points = append(f.stakes.points, app.StakePoint{
		CabalID: f.cabal, At: at.Add(time.Minute), ShareUnits: money.SharesUnitsFromUint64(0),
		NetContributed: money.SignedMicrosFromInt64(-1_000),
	})
	got, err := f.read(t, fakeBoards{run: f.run, hasRun: true})
	if err != nil || !got.NetWorth.IsZero() || got.Arriving.Int64() >= 0 {
		t.Fatalf(
			"Run = net worth %d arriving %d, %v, want 0 and a negative arriving",
			got.NetWorth.Uint64(),
			got.Arriving.Int64(),
			err,
		)
	}
}
