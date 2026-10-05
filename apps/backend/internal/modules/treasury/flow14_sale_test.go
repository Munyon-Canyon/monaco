package treasury_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
)

func saleAndPayout(t *testing.T, invested uint64) (*saleRig, *payoutRig) {
	t.Helper()
	s := newSaleRig(t, invested)
	return s, (&payoutRig{f: s.f, alice: s.alice, bob: s.bob, cabal: s.cabal, job: s.job}).stubs()
}

func (r *payoutRig) payLanded(t *testing.T) {
	t.Helper()
	r.payLandedAs(t, seededSig(1))
}

func (r *payoutRig) payLandedAs(t *testing.T, sig chain.Signature) {
	t.Helper()
	if _, err := r.f.pool.Exec(t.Context(), `INSERT INTO cash_out_payouts
		(job_id, attempt, signature, signed_tx, last_valid_block_height, status, created_at)
		VALUES ($1, 1, $2, '\x01', 100, $3, $4)`,
		r.job, sig, string(domain.PayoutBroadcast), r.f.clock.Now()); err != nil {
		t.Fatal(err)
	}
	r.chain.set(sig, landed())
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
	t.Run("a short Jupiter fill pays what it raised", func(t *testing.T) {
		t.Parallel()
		flows.F14CashOutPayoutsSaleShort(flow14(t))
	})
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
