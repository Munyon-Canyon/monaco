package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (r *payoutRig) payoutsWith(budgets relayer.SponsorBudgets) *app.CashOutPayouts {
	return app.NewCashOutPayouts(app.CashOutPayoutDeps{
		UoW: r.f.uow, Reads: r.f.pool, Ledger: r.f.ledger, IDs: r.f.ids, Clock: autoClock{r.f.clock},
		Chain: r.chain, USDC: chain.Mint{Address: usdcMint, Decimals: 6}, Hints: r.hints,
		Transfers: func() (app.PayoutTransfers, error) { return r.transfers, nil }, Wallets: r.wallets,
		Sponsor: budgets,
	})
}

func payoutCtx(r *payoutRig) context.Context {
	return observability.WithActor(r.f.ctx(), "system:treasury.cashout_payout")
}

func (r *payoutRig) reservations(t *testing.T) string {
	t.Helper()
	return r.scalar(t, `SELECT count(*)::text || ':' || count(*) FILTER (WHERE released_at IS NULL)::text
		FROM relayer_sponsor_reservations WHERE scope = 'user' AND scope_key = $1`, r.alice.String())
}

func TestCashOutPayout_createsTheMembersAccountWhileTheSponsorBudgetHolds(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.chain.set(payoutSig(1), landed())
	r.mustAdvance(t)
	if len(r.transfers.built) != 1 || r.transfers.built[0].SkipCreate {
		t.Fatalf("built %+v, want one transfer that creates the account", r.transfers.built)
	}
	if got := r.reservations(t); got != "1:1" {
		t.Fatalf("reservations = %s, want one live", got)
	}
}

func TestCashOutPayout_skipsAccountCreationOnceTheSponsorBudgetIsSpent(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	budgets := relayer.SponsorBudgets{UserDaily: relayer.NetworkFeeLamports}
	r.chain.set(payoutSig(1), landed())
	if err := r.payoutsWith(budgets).Advance(payoutCtx(r), r.job, 0, nil); err != nil {
		t.Fatal(err)
	}
	if len(r.transfers.built) != 1 || !r.transfers.built[0].SkipCreate {
		t.Fatalf("built %+v, want one transfer that skips account creation", r.transfers.built)
	}
}

func TestCashOutPayout_waitsWhenTheSponsorBudgetCannotEvenPayTheFee(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	budgets := relayer.SponsorBudgets{UserDaily: 1}
	err := r.payoutsWith(budgets).Advance(payoutCtx(r), r.job, 0, nil)
	if errs.CodeOf(err) != errs.CodeRateLimited {
		t.Fatalf("Advance = %v, want rate_limited", err)
	}
	r.wantJob(t, "started", "")
	if len(r.transfers.built) != 0 || r.shares(t, r.alice) != 0 {
		t.Fatalf("built %d, alice shares %d; nothing may be signed and no units returned",
			len(r.transfers.built), r.shares(t, r.alice))
	}
}

func TestCashOutPayout_aLapsedAttemptGivesItsSponsorReservationBack(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.chain.set(payoutSig(1), lapsed())
	r.chain.set(payoutSig(2), landed())
	r.mustAdvance(t)
	r.wantJob(t, "completed", "")
	if got := r.reservations(t); got != "2:1" {
		t.Fatalf("reservations = %s, want the lapsed attempt's released and the landed one's live", got)
	}
}

func TestCashOutPayout_aFailedJobGivesItsSponsorReservationBack(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.transfers.buildErr = errs.New(errs.CodeInvalidAddress, "test.build")
	r.mustAdvance(t)
	r.wantJob(t, "failed", string(errs.CodePayoutFailed))
	if got := r.reservations(t); got != "1:0" {
		t.Fatalf("reservations = %s, want the only one released", got)
	}
}

func TestCashOutSweeper_waitsOutTheBackoffBetweenLapsedAttempts(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	sweeper := adapters.CashOutSweeper{Payouts: r.payouts()}
	ctx := observability.WithActor(r.f.ctx(), "system:poller.treasury.cashout-sweeper")
	for n := 1; n <= 4; n++ {
		r.seed(t, n, domain.PayoutExpired)
	}
	r.f.clock.Advance(domain.PayoutRetryBase*8 - time.Second)
	if report, err := sweeper.Tick(ctx); err != nil || report.Scanned != 0 {
		t.Fatalf("inside the backoff: report %+v, %v", report, err)
	}
	r.f.clock.Advance(2 * time.Second)
	r.chain.set(seededSig(5), landed())
	if report, err := sweeper.Tick(ctx); err != nil || report.Scanned != 1 {
		t.Fatalf("after the backoff: report %+v, %v", report, err)
	}
	if len(r.transfers.built) != 1 {
		t.Fatalf("built %d, want the next attempt signed", len(r.transfers.built))
	}
}

func TestCashOutSweeper_capsTheBackoff(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	sweeper := adapters.CashOutSweeper{Payouts: r.payouts()}
	ctx := observability.WithActor(r.f.ctx(), "system:poller.treasury.cashout-sweeper")
	for n := 1; n < domain.MaxPayoutAttempts; n++ {
		r.seed(t, n, domain.PayoutExpired)
	}
	r.f.clock.Advance(domain.PayoutRetryCeiling + time.Second)
	if report, err := sweeper.Tick(ctx); err != nil || report.Scanned != 1 {
		t.Fatalf("after the ceiling: report %+v, %v", report, err)
	}
}

func TestCashOutSweeper_sendsAStuckJobToTheBackOfTheLine(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	sweeper := adapters.CashOutSweeper{Payouts: r.payouts()}
	ctx := observability.WithActor(r.f.ctx(), "system:poller.treasury.cashout-sweeper")
	r.f.clock.Advance(app.CashOutSweepStale + time.Second)
	r.chain.err = errs.New(errs.CodeRPCUnavailable, "test.rpc")
	r.transfers.buildErr = errs.New(errs.CodePrivyUnavailable, "test.sign")
	before := r.scalar(t, `SELECT updated_at::text FROM cash_out_jobs WHERE id = $1`, r.job)
	if _, err := sweeper.Tick(ctx); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("Tick = %v, want privy_unavailable", err)
	}
	after := r.scalar(t, `SELECT updated_at::text FROM cash_out_jobs WHERE id = $1`, r.job)
	if after == before {
		t.Fatalf("updated_at stayed %s; a stuck job must rotate behind the others", before)
	}
	r.wantJob(t, "started", "")
}

func TestCashOutPayout_aSponsorReleaseFailureKeepsTheJobPaying(t *testing.T) {
	t.Parallel()
	for name, reading := range map[string]domain.PayoutReading{"lapse": lapsed(), "rejection": refused()} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newPayoutRig(t)
			r.mustAdvance(t)
			if _, err := r.f.pool.Exec(t.Context(),
				refuse("relayer_sponsor_reservations", "NEW.released_at IS NOT NULL")); err != nil {
				t.Fatal(err)
			}
			r.chain.set(payoutSig(1), reading)
			if err := r.advance(t, 0); err == nil {
				t.Fatal("Advance succeeded although the sponsor reservation could not be released")
			}
			r.wantJob(t, "paying", "")
			if got := r.attempts(t); len(got) != 1 || got[0] != "1:broadcast" {
				t.Fatalf("attempts = %v, want the attempt untouched", got)
			}
		})
	}
}
