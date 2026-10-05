package treasury_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func TestCashOutPayoutConsumer_paysAndRecordsTheDelivery(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.chain.set(payoutSig(1), landed())
	var event uuid.UUID
	if err := r.f.pool.QueryRow(t.Context(), `SELECT id FROM events WHERE aggregate_id = $1`, r.job).
		Scan(&event); err != nil {
		t.Fatal(err)
	}
	d := bus.Delivery{Handler: "treasury.cashout_payout", EventID: ids.EventIDFrom(event), At: r.f.clock.Now()}
	consumer := adapters.CashOutPayout{Payouts: r.payouts(), UoW: r.f.uow}
	if err := consumer.Handle(r.f.ctx(), d, events.CashOutStarted{V: 1, JobID: r.job}); err != nil {
		t.Fatal(err)
	}
	r.wantJob(t, "completed", "")
	if r.f.count(t, "event_deliveries") != 1 {
		t.Fatal("the delivery was not recorded")
	}
	r.chain.err = errs.New(errs.CodeRPCUnavailable, "test.rpc")
	r.job = r.f.ids.NewV7()
	if err := consumer.Handle(r.f.ctx(), d, events.CashOutStarted{V: 1, JobID: r.job}); err == nil {
		t.Fatal("an unknown job was acked")
	}
}

func TestCashOutSweeper_drivesDueJobsAndLeavesFreshOnes(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	sweeper := adapters.CashOutSweeper{Payouts: r.payouts()}
	ctx := observability.WithActor(r.f.ctx(), "system:poller.treasury.cashout-sweeper")
	if sweeper.Name() != "treasury.cashout-sweeper" || sweeper.Interval() != app.CashOutSweepInterval {
		t.Fatalf("sweeper = %s every %s", sweeper.Name(), sweeper.Interval())
	}
	report, err := sweeper.Tick(ctx)
	if err != nil || report.Scanned != 0 {
		t.Fatalf("fresh job: report %+v, %v", report, err)
	}
	r.f.clock.Advance(app.CashOutSweepStale + time.Second)
	if report, err = sweeper.Tick(ctx); err != nil || report.Scanned != 1 {
		t.Fatalf("stale job: report %+v, %v", report, err)
	}
	r.wantJob(t, "paying", "")
	if report, err = sweeper.Tick(ctx); err != nil || report.Scanned != 0 {
		t.Fatalf("young attempt: report %+v, %v", report, err)
	}
	r.f.clock.Advance(app.CashOutSweepStale + time.Second)
	r.chain.set(payoutSig(1), landed())
	if _, err := sweeper.Tick(ctx); err != nil {
		t.Fatal(err)
	}
	r.wantJob(t, "completed", "")
	r.chain.err = errs.New(errs.CodeRPCUnavailable, "test.rpc")
	if _, err := r.f.pool.Exec(
		t.Context(),
		`UPDATE cash_out_jobs SET status = 'paying' WHERE id = $1`,
		r.job,
	); err != nil {
		t.Fatal(err)
	}
	if _, err := sweeper.Tick(ctx); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("Tick = %v, want rpc_unavailable", err)
	}
}

func TestCashOutSweeper_aListFailureIsInternal(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	if _, err := r.f.pool.Exec(
		t.Context(),
		`ALTER TABLE cash_out_payouts RENAME TO cash_out_payouts_gone`,
	); err != nil {
		t.Fatal(err)
	}
	sweeper := adapters.CashOutSweeper{Payouts: r.payouts()}
	if _, err := sweeper.Tick(r.f.ctx()); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Tick = %v, want internal", err)
	}
}
