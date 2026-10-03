package treasury_test

import (
	"context"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestStakeProperty_valuesNeverExceedPot(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.cabal(t)
	users := make([]ids.UserID, 6)
	for i := range users {
		users[i] = f.user(t)
	}
	u, c, err := f.fund(users[0], cabal, 100_000_003, 101, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	swap, err := f.swap(cabal, 33_333_334, 111_111_113)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postCabal(swap); err != nil {
		t.Fatal(err)
	}
	prices := &marketfake.PricesFake{}
	prices.Set(marketfake.AAPLx().ID, money.MicrosFromUint64(30_000_001), f.clock.Now())
	q := adapterQueries(f, prices)
	rapid.Check(t, func(rt *rapid.T) {
		user := rapid.SampledFrom(users).Draw(rt, "user")
		micros := rapid.Int64Range(1, 1_000_000).Draw(rt, "micros")
		shares := rapid.Int64Range(1, 1_000_000).Draw(rt, "shares")
		postRandomFund(rt, f, user, cabal, micros, shares)
		assertStakeValues(rt, q, cabal, users)
	})
}

func postRandomFund(rt *rapid.T, f fixture, user ids.UserID, cabal ids.CabalID, micros, shares int64) {
	u, c, err := f.fund(user, cabal, micros, shares, domain.TxnSettled)
	if err != nil {
		rt.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		rt.Fatal(err)
	}
}

func assertStakeValues(rt *rapid.T, q *adapters.Queries, cabal ids.CabalID, users []ids.UserID) {
	pot, err := q.PotValue(context.Background(), cabal)
	if err != nil {
		rt.Fatal(err)
	}
	var total money.Micros
	for _, member := range users {
		units, err := q.ShareUnits(context.Background(), cabal, member)
		if err != nil || units.IsZero() {
			continue
		}
		stake, err := q.Stake(context.Background(), cabal, member)
		if err != nil {
			rt.Fatal(err)
		}
		want, err := domain.PayoutFor(stake.ShareUnits, stake.TotalShares, pot)
		if err != nil || stake.ValueMicros != want {
			rt.Fatalf("Stake() = %v, want %v, err %v", stake.ValueMicros, want, err)
		}
		total, err = total.Add(stake.ValueMicros)
		if err != nil {
			rt.Fatal(err)
		}
	}
	if total.Uint64() > pot.Uint64() {
		rt.Fatalf("stake total %v exceeds pot %v", total, pot)
	}
}
