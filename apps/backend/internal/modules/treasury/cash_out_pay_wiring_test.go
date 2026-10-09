package treasury_test

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
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

func TestCashOutSweeper_finishesAPayoutThatLandsAfterTheConsumerGaveUp(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.chain.set(payoutSig(1), processing())
	if err := r.advance(t, app.CashOutPayoutWait); err != nil {
		t.Fatal(err)
	}
	r.wantJob(t, "paying", "")
	if got := r.attempts(t); !slices.Equal(got, []string{"1:broadcast"}) {
		t.Fatalf("attempts = %v, want the payout broadcast and unconfirmed", got)
	}
	r.chain.set(payoutSig(1), landed())
	r.f.clock.Advance(app.CashOutSweepInterval)
	ctx := observability.WithActor(r.f.ctx(), "system:poller.treasury.cashout-sweeper")
	report, err := adapters.CashOutSweeper{Payouts: r.payouts()}.Tick(ctx)
	if err != nil || report.Scanned != 1 {
		t.Fatalf("first sweep after the consumer gave up: report %+v, %v", report, err)
	}
	r.wantJob(t, "completed", "")
	r.settledOnce(t)
}

type overlap struct {
	chain        *payoutChain
	transfers    *payoutTransfers
	at           string
	held, resume chan struct{}
	taken        atomic.Bool
}

func (o *overlap) hold(ctx context.Context, at string) {
	if at != o.at || !o.taken.CompareAndSwap(false, true) {
		return
	}
	close(o.held)
	select {
	case <-o.resume:
	case <-ctx.Done():
	}
}

func (o *overlap) Build(ctx context.Context, spec relayer.TransferSpec) (relayer.SignedTx, error) {
	signed, err := o.transfers.Build(ctx, spec)
	o.hold(ctx, "build")
	return signed, err
}

func (o *overlap) Broadcast(ctx context.Context, tx relayer.SignedTx) error {
	err := o.transfers.Broadcast(ctx, tx)
	o.hold(ctx, "broadcast")
	return err
}

func (o *overlap) PayoutStatus(
	ctx context.Context, sig chain.Signature, lastValid uint64,
) (domain.PayoutReading, error) {
	reading, err := o.chain.PayoutStatus(ctx, sig, lastValid)
	o.hold(ctx, "status")
	return reading, err
}

func (r *payoutRig) consumerHeldAt(o *overlap) *app.CashOutPayouts {
	return app.NewCashOutPayouts(app.CashOutPayoutDeps{
		UoW: r.f.uow, Reads: r.f.pool, Ledger: r.f.ledger, IDs: r.f.ids, Clock: autoClock{r.f.clock}, Chain: o,
		USDC: chain.Mint{Address: usdcMint, Decimals: 6}, Hints: r.hints, Wallets: r.wallets,
		Transfers: func() (app.PayoutTransfers, error) { return o, nil },
	})
}

func (r *payoutRig) sweepWhileConsumerHeldAt(t *testing.T, at string) {
	t.Helper()
	o := &overlap{
		chain: r.chain, transfers: r.transfers, at: at, held: make(chan struct{}), resume: make(chan struct{}),
	}
	var consumer errgroup.Group
	ended := make(chan struct{})
	consumer.Go(func() error {
		defer close(ended)
		ctx := observability.WithActor(r.f.ctx(), "system:treasury.cashout_payout")
		return r.consumerHeldAt(o).Advance(ctx, r.job, app.CashOutPayoutWait, nil)
	})
	select {
	case <-o.held:
	case <-ended:
		t.Fatalf("the consumer ended before reaching %s: %v", at, consumer.Wait())
	}
	r.f.clock.Advance(app.CashOutSweepStale + time.Second)
	ctx := observability.WithActor(r.f.ctx(), "system:poller.treasury.cashout-sweeper")
	if report, err := (adapters.CashOutSweeper{Payouts: r.payouts()}).Tick(ctx); err != nil || report.Scanned != 1 {
		t.Fatalf("sweep: report %+v, %v", report, err)
	}
	r.wantJob(t, "completed", "")
	close(o.resume)
	if err := consumer.Wait(); err != nil {
		t.Fatalf("the consumer resumed into %v", err)
	}
}

func (r *payoutRig) settledOnce(t *testing.T) {
	t.Helper()
	if r.events(t, events.TypeCashOutCompleted) != "1" || r.events(t, events.TypeCashOutFailed) != "0" {
		t.Fatalf("completed events %s, failed events %s, want 1 and 0",
			r.events(t, events.TypeCashOutCompleted), r.events(t, events.TypeCashOutFailed))
	}
	if r.treasuryUSDC(t) != "50000000" || r.transferStatuses(t) != "cabal=settled user=settled" {
		t.Fatalf("treasury %s USDC, headers %s", r.treasuryUSDC(t), r.transferStatuses(t))
	}
	if r.shares(t, r.alice) != 0 {
		t.Fatalf("alice holds %d share units after a paid cash out", r.shares(t, r.alice))
	}
	r.noDrift(t)
}

func TestCashOutSweeper_aSweepThatOverlapsTheConsumerPaysOnce(t *testing.T) {
	t.Parallel()
	type readings = map[chain.Signature]domain.PayoutReading
	for _, c := range []struct {
		name, at string
		seed     domain.PayoutStatus
		readings readings
		attempts []string
		built    int
		sent     []chain.Signature
	}{
		{
			name: "signing", at: "build", readings: readings{payoutSig(2): landed()},
			attempts: []string{"1:confirmed"}, built: 2, sent: []chain.Signature{payoutSig(2)},
		},
		{
			name: "sending", at: "broadcast", seed: domain.PayoutSigned, readings: readings{seededSig(1): landed()},
			attempts: []string{"1:confirmed"}, sent: []chain.Signature{seededSig(1), seededSig(1)},
		},
		{
			name: "completing", at: "status", seed: domain.PayoutBroadcast, readings: readings{seededSig(1): landed()},
			attempts: []string{"1:confirmed"},
		},
		{
			name: "lapsing", at: "status", seed: domain.PayoutBroadcast,
			readings: readings{seededSig(1): lapsed(), payoutSig(1): landed()},
			attempts: []string{"1:expired", "2:confirmed"}, built: 1, sent: []chain.Signature{payoutSig(1)},
		},
	} {
		t.Run(c.name, func(t *testing.T) {
			t.Parallel()
			r := newPayoutRig(t)
			if c.seed != "" {
				r.seed(t, 1, c.seed)
			}
			for sig, reading := range c.readings {
				r.chain.set(sig, reading)
			}
			r.sweepWhileConsumerHeldAt(t, c.at)
			if got := r.attempts(t); !slices.Equal(got, c.attempts) {
				t.Fatalf("attempts = %v, want %v", got, c.attempts)
			}
			if len(r.transfers.built) != c.built || !slices.Equal(r.transfers.sent, c.sent) {
				t.Fatalf("built %d transfers and sent %v, want %d built and %v sent",
					len(r.transfers.built), r.transfers.sent, c.built, c.sent)
			}
			r.settledOnce(t)
		})
	}
}
