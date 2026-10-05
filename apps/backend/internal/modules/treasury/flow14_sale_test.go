package treasury_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
)

func saleAndPayout(t *testing.T, invested uint64) (*saleRig, *payoutRig) {
	t.Helper()
	s := newSaleRig(t, invested)
	return s, (&payoutRig{f: s.f, alice: s.alice, bob: s.bob, cabal: s.cabal, job: s.job}).stubs()
}

func (r *payoutRig) payLanded(t *testing.T) {
	t.Helper()
	r.seed(t, 1, domain.PayoutBroadcast)
	r.chain.set(seededSig(1), landed())
	r.mustAdvance(t)
}

func (r *payoutRig) wantEnded(t *testing.T, want events.Type) {
	t.Helper()
	for _, typ := range []events.Type{events.TypeCashOutCompleted, events.TypeCashOutPartial, events.TypeCashOutFailed} {
		n := "0"
		if typ == want {
			n = "1"
		}
		if got := r.events(t, typ); got != n {
			t.Fatalf("%s events = %s, want %s", typ, got, n)
		}
	}
}

func TestCashOutSale_shortProceedsPayPartialAndNoProceedsFail(t *testing.T) {
	t.Parallel()
	t.Run("partial proceeds pay what the sale raised and return the uncovered units", func(t *testing.T) {
		t.Parallel()
		s, r := saleAndPayout(t, 80_000_000)
		s.deliver(t, s.started)
		s.deliver(t, s.sold(s.f.ids.NewV7(), 400, 15_000_000, 2))
		s.deliver(t, s.unsold(s.f.ids.NewV7(), 200, 2))
		s.want(t, "paying", "35000000", "30", "sale_short", 30)
		r.payLanded(t)
		r.wantJob(t, "partial", "sale_short")
		r.wantEnded(t, events.TypeCashOutPartial)
		if r.shares(t, s.alice) != 30 {
			t.Fatalf("alice holds %d shares, want the 30 the sale did not cover", r.shares(t, s.alice))
		}
		r.noDrift(t)
	})
	t.Run("zero proceeds fail the job and return every unit", func(t *testing.T) {
		t.Parallel()
		s, r := saleAndPayout(t, 100_000_000)
		s.deliver(t, s.started)
		s.deliver(t, s.unsold(s.f.ids.NewV7(), 500, 1))
		s.want(t, "failed", "50000000", "100", "sale_short", 100)
		r.wantEnded(t, events.TypeCashOutFailed)
		r.noDrift(t)
	})
}

func TestCashOutSale_aTradeConfirmedBeforeSellingIsKeptAndTheJobCompletes(t *testing.T) {
	t.Parallel()
	s, r := saleAndPayout(t, 80_000_000)
	s.deliver(t, s.sold(s.f.ids.NewV7(), 600, 31_000_000, 1))
	s.want(t, "started", "50000000", "0", "", 0)
	s.deliver(t, s.started)
	s.want(t, "paying", "50000000", "0", "", 0)
	r.payLanded(t)
	r.wantJob(t, "completed", "")
	r.wantEnded(t, events.TypeCashOutCompleted)
	r.noDrift(t)
}
