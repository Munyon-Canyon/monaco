package treasury_test

import (
	"strconv"
	"testing"

	"pgregory.net/rapid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

const (
	propertySlice = 50_000_000
	propertyUnits = 100
)

func TestCashOutSaleProperty_neverOverpaysKeepsEveryUnitAndBalancesTheLedgers(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	rapid.Check(t, func(rt *rapid.T) {
		invested := rapid.Uint64Range(propertySlice+1, 100_000_000).Draw(rt, "invested")
		s := newSaleRigOn(t, f, invested)
		r := (&payoutRig{f: s.f, alice: s.alice, bob: s.bob, cabal: s.cabal, job: s.job}).stubs()
		s.deliver(t, s.started)
		legs := rapid.IntRange(1, 3).Draw(rt, "legs")
		for range legs {
			if rapid.Bool().Draw(rt, "filled") {
				s.deliver(t, s.sold(f.ids.NewV7(), 1, rapid.Uint64Range(1, invested).Draw(rt, "out"), legs))
			} else {
				s.deliver(t, s.unsold(f.ids.NewV7(), 1, legs))
			}
		}
		if s.state(t).Status == "paying" {
			r.payLandedAs(t, chain.Signature("property-"+s.job.String()))
		}
		checkSaleInvariants(rt, s, r)
	})
}

func checkSaleInvariants(rt *rapid.T, s *saleRig, r *payoutRig) {
	t := s.f.t
	got := s.state(t)
	paid, _ := strconv.ParseUint(got.Payout, 10, 64)
	returned, _ := strconv.ParseUint(got.Returned, 10, 64)
	held := s.shares(t, s.alice)
	switch got.Status {
	case "completed", "partial":
		if paid > propertySlice || returned != propertyUnits*(propertySlice-paid)/propertySlice {
			rt.Fatalf("%s job paid %d and returned %d units, want at most the slice and the floor of the rest",
				got.Status, paid, returned)
		}
	case "failed":
		if returned != propertyUnits || r.events(t, events.TypeCashOutFailed) != "1" {
			rt.Fatalf("failed job returned %d units with %s failures", returned, r.events(t, events.TypeCashOutFailed))
		}
	default:
		rt.Fatalf("job stopped in %s", got.Status)
	}
	if held != returned {
		rt.Fatalf("alice holds %d units, want the %d returned so burned plus returned is %d", held, returned,
			propertyUnits)
	}
	if drift := s.f.drift(t); len(drift) != 0 {
		rt.Fatalf("ledger drift = %#v", drift)
	}
}
