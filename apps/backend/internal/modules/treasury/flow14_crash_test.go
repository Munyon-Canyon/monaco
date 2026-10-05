//go:build faultpoints

package treasury_test

import (
	"context"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (r *payoutRig) crashPaying(t *testing.T, point faultpoint.Name) {
	t.Helper()
	testkit.CrashAt(t, point, func(ctx context.Context) error {
		return r.payouts().Advance(observability.WithActor(ctx, "system:treasury.cashout_payout"), r.job, 0, nil)
	})
	r.mustAdvance(t)
}

func (r *payoutRig) paidOnce(t *testing.T, built int, sent ...chain.Signature) {
	t.Helper()
	r.wantJob(t, "completed", "")
	if got := r.attempts(t); !slices.Equal(got, []string{"1:confirmed"}) {
		t.Fatalf("attempts = %v, want one confirmed payout", got)
	}
	if len(r.transfers.built) != built || !slices.Equal(r.transfers.sent, sent) {
		t.Fatalf("built %d transfers and sent %v, want %d built and %v sent", len(r.transfers.built),
			r.transfers.sent, built, sent)
	}
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

func TestFlow14_CashOutPayouts_CrashAfterSign(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.chain.set(payoutSig(2), landed())
	r.crashPaying(t, faultpoint.AfterSign)
	r.paidOnce(t, 2, payoutSig(2))
}

func TestFlow14_CashOutPayouts_CrashAfterBroadcast(t *testing.T) {
	t.Parallel()
	r := newPayoutRig(t)
	r.chain.set(payoutSig(1), landed())
	r.crashPaying(t, faultpoint.AfterBroadcast)
	r.paidOnce(t, 1, payoutSig(1), payoutSig(1))
}

func TestFlow14_CashOutPayouts_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	t.Run("storing the signed payout", func(t *testing.T) {
		t.Parallel()
		r := newPayoutRig(t)
		r.chain.set(payoutSig(2), landed())
		r.crashPaying(t, faultpoint.BeforeCommit)
		r.paidOnce(t, 2, payoutSig(2))
	})
	t.Run("settling the landed payout", func(t *testing.T) {
		t.Parallel()
		r := newPayoutRig(t)
		r.mustAdvance(t)
		if got := r.attempts(t); !slices.Equal(got, []string{"1:broadcast"}) {
			t.Fatalf("attempts = %v before the settling crash, want one broadcast payout", got)
		}
		r.chain.set(payoutSig(1), landed())
		r.crashPaying(t, faultpoint.BeforeCommit)
		r.paidOnce(t, 1, payoutSig(1))
	})
}
