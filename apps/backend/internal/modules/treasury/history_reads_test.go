package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type historyCase struct {
	cabal         ids.CabalID
	user          ids.UserID
	contributions []port.ContributionPoint
	stakes        []port.StakePoint
}

func historyWant(cabal ids.CabalID, user ids.UserID, at []time.Time) historyCase {
	net := money.SignedMicrosFromInt64
	units := money.SharesUnitsFromUint64
	return historyCase{
		cabal: cabal, user: user,
		contributions: []port.ContributionPoint{
			{At: at[0], NetContributed: net(100_000_000)},
			{At: at[1], NetContributed: net(150_000_000)},
			{At: at[2], NetContributed: net(125_000_000)},
		},
		stakes: []port.StakePoint{
			{CabalID: cabal, At: at[0], ShareUnits: units(100), NetContributed: net(100_000_000)},
			{CabalID: cabal, At: at[1], ShareUnits: units(150), NetContributed: net(150_000_000)},
			{CabalID: cabal, At: at[2], ShareUnits: units(125), NetContributed: net(125_000_000)},
		},
	}
}

func assertContributions(t *testing.T, got, want []port.ContributionPoint) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("contributions = %#v, want %#v", got, want)
	}
	for i, point := range got {
		if !point.At.Equal(want[i].At) || point.NetContributed != want[i].NetContributed {
			t.Fatalf("contribution %d = %#v, want %#v", i, point, want[i])
		}
	}
}

func assertStakes(t *testing.T, got, want []port.StakePoint) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatalf("stakes = %#v, want %#v", got, want)
	}
	for i, point := range got {
		w := want[i]
		if point.CabalID != w.CabalID || !point.At.Equal(w.At) || point.ShareUnits != w.ShareUnits ||
			point.NetContributed != w.NetContributed {
			t.Fatalf("stake %d = %#v, want %#v", i, point, w)
		}
	}
}

func assertEmptyHistory(t *testing.T, reader port.HistoryReader) {
	t.Helper()
	contributions, err := reader.CabalContributionHistory(t.Context(), ids.CabalIDFrom(ids.Real{}.NewV7()))
	if err != nil || contributions == nil || len(contributions) != 0 {
		t.Fatalf("empty contributions = %#v, %v", contributions, err)
	}
	stakes, err := reader.UserStakeHistory(t.Context(), ids.UserIDFrom(ids.Real{}.NewV7()))
	if err != nil || stakes == nil || len(stakes) != 0 {
		t.Fatalf("empty stakes = %#v, %v", stakes, err)
	}
}

func assertHistoryContract(t *testing.T, reader port.HistoryReader, want historyCase) {
	t.Helper()
	contributions, err := reader.CabalContributionHistory(t.Context(), want.cabal)
	if err != nil {
		t.Fatal(err)
	}
	assertContributions(t, contributions, want.contributions)
	stakes, err := reader.UserStakeHistory(t.Context(), want.user)
	if err != nil {
		t.Fatal(err)
	}
	assertStakes(t, stakes, want.stakes)
	assertEmptyHistory(t, reader)
}

func TestHistoryReads_AdapterAndFakeShareAContract(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	at := make([]time.Time, 0, 3)
	mustFundHistory(t, f, user, cabal, 100_000_000, 100)
	at = append(at, f.clock.Now())
	f.clock.Advance(time.Second)
	mustFundHistory(t, f, user, cabal, 50_000_000, 50)
	at = append(at, f.clock.Now())
	f.clock.Advance(time.Second)
	mustCashOutHistory(t, f, user, cabal, 25_000_000, 25)
	at = append(at, f.clock.Now())
	want := historyWant(cabal, user, at)
	t.Run("adapter", func(t *testing.T) {
		t.Parallel()
		assertHistoryContract(t, newQueries(f), want)
	})
	t.Run("fake", func(t *testing.T) {
		t.Parallel()
		fake := fakes.NewTreasury()
		fake.SetCabalContributionHistory(cabal, want.contributions)
		fake.SetUserStakeHistory(user, want.stakes)
		assertHistoryContract(t, fake, want)
	})
}

func assertLastPointMatchesStake(t *testing.T, q port.Queries, point port.StakePoint, user ids.UserID) {
	t.Helper()
	stake, err := q.Stake(t.Context(), point.CabalID, user)
	if err != nil {
		t.Fatal(err)
	}
	net, err := stake.ContributedMicros.Delta(stake.WithdrawnMicros)
	if err != nil {
		t.Fatal(err)
	}
	if point.ShareUnits != stake.ShareUnits || point.NetContributed != net {
		t.Fatalf("last point %#v, stake %#v net %v", point, stake, net)
	}
}

func TestHistoryReads_LastPointMatchesStakeAndPotsSplitByCabal(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, other, cabal, otherCabal := f.user(t), f.user(t), f.cabal(t), f.cabal(t)
	mustFundHistory(t, f, user, cabal, 100_000_000, 100)
	f.clock.Advance(time.Second)
	mustFundHistory(t, f, other, cabal, 40_000_000, 40)
	f.clock.Advance(time.Second)
	mustFundHistory(t, f, user, otherCabal, 7_000_000, 7)
	f.clock.Advance(time.Second)
	mustCashOutHistory(t, f, user, cabal, 30_000_000, 30)
	q := newQueries(f)
	stakes, err := q.UserStakeHistory(t.Context(), user)
	if err != nil || len(stakes) != 3 {
		t.Fatalf("UserStakeHistory = %#v, %v", stakes, err)
	}
	if !stakes[0].At.Before(stakes[1].At) || !stakes[1].At.Before(stakes[2].At) {
		t.Fatalf("stakes out of order: %#v", stakes)
	}
	assertLastPointMatchesStake(t, q, stakes[2], user)
	assertLastPointMatchesStake(t, q, stakes[1], user)
	contributions, err := q.CabalContributionHistory(t.Context(), cabal)
	if err != nil || len(contributions) != 3 {
		t.Fatalf("CabalContributionHistory = %#v, %v", contributions, err)
	}
	if got := contributions[2].NetContributed.Int64(); got != 110_000_000 {
		t.Fatalf("last cabal contribution = %d, want 110000000", got)
	}
}

func TestHistoryReads_OneQueryEach(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	mustFundHistory(t, f, user, cabal, 100_000_000, 100)
	q := newQueries(f)
	testkit.AssertQueries(t, "CabalContributionHistory", func() {
		if _, err := q.CabalContributionHistory(t.Context(), cabal); err != nil {
			t.Fatal(err)
		}
	})
	testkit.AssertQueries(t, "UserStakeHistory", func() {
		if _, err := q.UserStakeHistory(t.Context(), user); err != nil {
			t.Fatal(err)
		}
	})
}

func TestHistoryReads_CanceledReadsFail(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	q := newQueries(f)
	if _, err := q.CabalContributionHistory(ctx, f.cabal(t)); err == nil {
		t.Fatal("CabalContributionHistory error = nil")
	}
	if _, err := q.UserStakeHistory(ctx, f.user(t)); err == nil {
		t.Fatal("UserStakeHistory error = nil")
	}
}

func TestHistoryReads_FakeFaults(t *testing.T) {
	t.Parallel()
	fake := fakes.NewTreasury()
	boom := errs.New(errs.CodeInternal, "test")
	fake.Fail("CabalContributionHistory", boom)
	fake.Fail("UserStakeHistory", boom)
	if _, err := fake.CabalContributionHistory(t.Context(), ids.CabalIDFrom(ids.Real{}.NewV7())); err == nil {
		t.Fatal("CabalContributionHistory error = nil")
	}
	if _, err := fake.UserStakeHistory(t.Context(), ids.UserIDFrom(ids.Real{}.NewV7())); err == nil {
		t.Fatal("UserStakeHistory error = nil")
	}
}
