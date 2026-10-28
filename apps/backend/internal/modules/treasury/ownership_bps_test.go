package treasury_test

import (
	"cmp"
	"maps"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (f fixture) queries() *adapters.Queries {
	return adapters.NewQueries(f.pool, nil, nil, f.clock, chain.SolanaAddress(f.cfg.Solana.USDCMint))
}

func (f fixture) holds(t *testing.T, user ids.UserID, cabal ids.CabalID, units int64) {
	t.Helper()
	u, c, err := f.fund(user, cabal, units, units, domain.TxnSettled)
	if err == nil {
		err = f.postPair(u, c)
	}
	if err != nil {
		t.Fatal(err)
	}
}

func TestOwnershipBps_isThePotsSliceForEveryHolder(t *testing.T) {
	t.Parallel()
	for name, tt := range map[string]struct {
		units []int64
		want  []int32
	}{
		"uneven, the largest takes the remainder": {
			units: []int64{33_333_333, 35_000_000, 31_666_667}, want: []int32{3333, 3501, 3166},
		},
		"a tie gives the remainder to the lower user id": {
			units: []int64{1_000_000, 1_000_000, 1_000_000}, want: []int32{3334, 3333, 3333},
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			cabal := f.cabal(t)
			users := []ids.UserID{f.user(t), f.user(t), f.user(t)}
			slices.SortFunc(users, func(a, b ids.UserID) int { return cmp.Compare(a.String(), b.String()) })
			for i, user := range users {
				f.holds(t, user, cabal, tt.units[i])
			}
			for i, user := range users {
				assertOwnershipIsPotSlice(t, f, cabal, user, tt.want[i])
			}
		})
	}
}

func assertOwnershipIsPotSlice(t *testing.T, f fixture, cabal ids.CabalID, user ids.UserID, want int32) {
	t.Helper()
	q := f.queries()
	owned, err := q.OwnershipBps(f.ctx(), user)
	if err != nil {
		t.Fatal(err)
	}
	pot, err := q.CabalPot(f.ctx(), cabal, user, true)
	if err != nil || pot.Me == nil || owned[cabal] != pot.Me.SliceBps || owned[cabal] != want {
		t.Fatalf("ownership = %d, pot me = %+v, %v, want both %d", owned[cabal], pot.Me, err, want)
	}
}

func TestOwnershipBps_coversOnlyTheCabalsTheUserHolds(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	me, other := f.user(t), f.user(t)
	solo, shared, left, theirs := f.cabal(t), f.cabal(t), f.cabal(t), f.cabal(t)
	f.holds(t, me, solo, 5_000_000)
	f.holds(t, me, shared, 1_000_000)
	f.holds(t, other, shared, 3_000_000)
	f.holds(t, me, left, 2_000_000)
	f.holds(t, other, left, 2_000_000)
	f.holds(t, other, theirs, 1_000_000)
	u, c, err := f.cashOut(me, left, 2_000_000, 2_000_000, domain.TxnSettled)
	if err == nil {
		err = f.postPair(u, c)
	}
	if err != nil {
		t.Fatal(err)
	}
	got, err := f.queries().OwnershipBps(f.ctx(), me)
	want := map[ids.CabalID]int32{solo: 10000, shared: 2500}
	if err != nil || !maps.Equal(got, want) {
		t.Fatalf("OwnershipBps = %v, %v, want %v without the cashed-out or never-held cabals", got, err, want)
	}
}
