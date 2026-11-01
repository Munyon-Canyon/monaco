package treasury_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (r *saleRig) sweep(t *testing.T) int {
	t.Helper()
	p := app.NewCashOutPayouts(app.CashOutPayoutDeps{
		UoW: r.f.uow, Reads: r.f.pool, Ledger: r.f.ledger, IDs: r.f.ids, Clock: r.f.clock,
	})
	n, err := p.SweepSales(observability.WithActor(r.f.ctx(), "system:treasury.cashout_sweeper"))
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *saleRig) startEvents(t *testing.T) int {
	t.Helper()
	var n int
	if err := r.f.pool.QueryRow(t.Context(), `SELECT count(*) FROM events WHERE type = 'cashout.started'
		AND aggregate_id = $1`, r.job).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func (r *saleRig) liveSwap(t *testing.T, status string) {
	t.Helper()
	if _, err := r.f.pool.Exec(t.Context(), `INSERT INTO swaps (id, source_kind, source_id, cabal_id, treasury_address,
		action, symbol, in_mint, out_mint, out_decimals, in_amount, slippage_bps, source_batch_size, status,
		created_at, updated_at)
		VALUES ($1, 'cashout', $2, $3, 'treasury', 'sell', 'AAPLx', 'aapl', 'usdc', 6, 1, 100, 1, $5, $4, $4)`,
		r.f.ids.NewV7(), r.job, r.cabal.UUID(), r.f.clock.Now(), status); err != nil {
		t.Fatal(err)
	}
}

func TestCashOutSaleStuck_aFreshSellingJobIsLeftAlone(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.deliver(t, r.started)
	r.f.clock.Advance(domain.CashOutSellStale - time.Minute)
	if n := r.sweep(t); n != 0 || r.startEvents(t) != 1 {
		t.Fatalf("swept %d with %d start events, want 0 and 1", n, r.startEvents(t))
	}
}

func TestCashOutSaleStuck_aSellingJobWithNoResultsGetsItsSaleReEmittedOnce(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.deliver(t, r.started)
	r.f.clock.Advance(domain.CashOutSellStale + time.Minute)
	if n := r.sweep(t); n != 1 {
		t.Fatalf("swept %d, want 1", n)
	}
	if got := r.startEvents(t); got != 2 {
		t.Fatalf("%d start events, want the original and one re-emit", got)
	}
	if n := r.sweep(t); n != 0 || r.startEvents(t) != 2 {
		t.Fatalf("second sweep touched %d jobs with %d start events, want 0 and 2", n, r.startEvents(t))
	}
	r.deliver(t, r.started)
	r.want(t, "selling", "50000000", "0", "", 0)
}

func TestCashOutSaleStuck_aStartedJobWhoseStartWasDroppedGetsItsSaleReEmitted(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.f.clock.Advance(domain.CashOutSellStale + time.Minute)
	if n := r.sweep(t); n != 1 || r.startEvents(t) != 2 {
		t.Fatalf("swept %d with %d start events, want 1 and 2", n, r.startEvents(t))
	}
	r.deliver(t, r.started)
	r.want(t, "selling", "50000000", "0", "", 0)
}

func TestCashOutSaleStuck_aLegStillInFlightIsNotTouched(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.deliver(t, r.started)
	r.liveSwap(t, "submitted")
	r.f.clock.Advance(domain.CashOutSellGiveUp + time.Minute)
	if n := r.sweep(t); n != 0 || r.startEvents(t) != 1 {
		t.Fatalf("swept %d with %d start events, want 0 and 1", n, r.startEvents(t))
	}
	r.want(t, "selling", "50000000", "0", "", 0)
}

func TestCashOutSaleStuck_aSaleThatNeverReportsFailsCleanlyAndReturnsEveryUnit(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 100_000_000)
	r.deliver(t, r.started)
	r.f.clock.Advance(domain.CashOutSellGiveUp + time.Minute)
	if n := r.sweep(t); n != 1 {
		t.Fatalf("swept %d, want 1", n)
	}
	r.want(t, "failed", "50000000", "100", "sale_short", 100)
	if r.originalTxn(t) != "failed" || r.failures(t) != 1 {
		t.Fatalf("original txn %s with %d failures, want it failed once", r.originalTxn(t), r.failures(t))
	}
	if n := r.sweep(t); n != 0 {
		t.Fatalf("second sweep touched %d jobs, want 0", n)
	}
}

func TestCashOutSaleStuck_aStartedJobThatNeverSoldFailsCleanly(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 100_000_000)
	r.f.clock.Advance(domain.CashOutSellGiveUp + time.Minute)
	if n := r.sweep(t); n != 1 {
		t.Fatalf("swept %d, want 1", n)
	}
	r.want(t, "failed", "50000000", "100", "sale_short", 100)
}

func TestCashOutSaleStuck_aSaleMissingALegPaysWhatItRaisedAndReturnsTheRest(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.deliver(t, r.started)
	r.deliver(t, r.sold(r.f.ids.NewV7(), 400, 15_000_000, 2))
	r.f.clock.Advance(domain.CashOutSellGiveUp + time.Minute)
	if n := r.sweep(t); n != 1 {
		t.Fatalf("swept %d, want 1", n)
	}
	r.want(t, "paying", "35000000", "30", "sale_short", 30)
	if r.originalTxn(t) != "pending" || r.failures(t) != 0 {
		t.Fatalf("original txn %s with %d failures, want it pending for the payout", r.originalTxn(t),
			r.failures(t))
	}
}

func TestCashOutSaleStuck_aFailedClaimIsReportedAndLeavesTheJobAlone(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	r.deliver(t, r.started)
	r.f.clock.Advance(domain.CashOutSellStale + time.Minute)
	if _, err := r.f.pool.Exec(t.Context(), refuse("cash_out_jobs", "true")); err != nil {
		t.Fatal(err)
	}
	p := app.NewCashOutPayouts(app.CashOutPayoutDeps{
		UoW: r.f.uow, Reads: r.f.pool, Ledger: r.f.ledger, IDs: r.f.ids, Clock: r.f.clock,
	})
	if _, err := p.SweepSales(observability.WithActor(r.f.ctx(), "system:treasury.cashout_sweeper")); err == nil {
		t.Fatal("SweepSales error = nil, want the refused claim")
	}
	if r.startEvents(t) != 1 {
		t.Fatalf("%d start events, want the refused claim to leave only the original", r.startEvents(t))
	}
}

func TestCashOutSaleStuck_aListFailureIsReported(t *testing.T) {
	t.Parallel()
	r := newSaleRig(t, 80_000_000)
	if _, err := r.f.pool.Exec(t.Context(), `DROP TABLE cash_out_jobs CASCADE`); err != nil {
		t.Fatal(err)
	}
	p := app.NewCashOutPayouts(app.CashOutPayoutDeps{
		UoW: r.f.uow, Reads: r.f.pool, Ledger: r.f.ledger, IDs: r.f.ids, Clock: r.f.clock,
	})
	if _, err := p.SweepSales(observability.WithActor(r.f.ctx(), "system:treasury.cashout_sweeper")); err == nil {
		t.Fatal("SweepSales error = nil, want the failed list")
	}
}
