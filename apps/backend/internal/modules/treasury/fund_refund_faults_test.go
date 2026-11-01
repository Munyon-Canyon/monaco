package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

var errDependency = errs.New(errs.CodeUpstreamUnavailable, "test.dependency")

func TestFundRefund_aFailingDependencyRefusesBeforeAnythingIsStored(t *testing.T) {
	t.Parallel()
	tests := []struct {
		name  string
		fault func(*refundStubs)
		code  errs.Code
	}{
		{"treasury wallet", func(s *refundStubs) { s.treasuryAt = errDependency }, errs.CodeUpstreamUnavailable},
		{"member wallet", func(s *refundStubs) { s.memberErr = errDependency }, errs.CodeUpstreamUnavailable},
		{"pot", func(s *refundStubs) { s.potErr = errDependency }, errs.CodeUpstreamUnavailable},
		{"balance", func(s *refundStubs) { s.balErr = errDependency }, errs.CodeUpstreamUnavailable},
		{"transfers", func(s *refundStubs) { s.factoryErr = errDependency }, errs.CodeUpstreamUnavailable},
		{"signer", func(s *refundStubs) { s.buildErr = errDependency }, errs.CodeUpstreamUnavailable},
		{"price back after signing", func(s *refundStubs) {
			s.onBuild = func() { s.priceable = true }
		}, errs.CodeFundRefundPriceable},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			t.Parallel()
			h := newRefundHarness(t)
			tt.fault(h.stubs)
			id := h.landed(t, "5000000")
			_, err := h.refund(id)
			wantCode(t, err, tt.code)
			if got := h.row(t, id); got.status != "landed" || got.signature != "" || len(h.stubs.broadcasts) != 0 ||
				h.count(t, "user_txns") != 0 {
				t.Fatalf(
					"row = %+v, broadcasts %d, want landed with nothing stored or sent",
					got,
					len(h.stubs.broadcasts),
				)
			}
		})
	}
}

func TestFundRefund_theChainReadingIsCheckedAfterTheSignatureIsStored(t *testing.T) {
	t.Parallel()
	for name, change := range map[string]func(*refundStubs){
		"status error": func(s *refundStubs) { s.statusErr = errDependency },
		"two rows":     func(s *refundStubs) { s.statusRows = 2 },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			h := newRefundHarness(t)
			change(h.stubs)
			id := h.landed(t, "5000000")
			got, err := h.refund(id)
			if err == nil || got.State == app.FundRefundDone {
				t.Fatalf("Refund = %+v, %v, want an error", got, err)
			}
			if row := h.row(t, id); row.status != "landed" || row.signature == "" {
				t.Fatalf("row = %+v, want landed with the signature kept for the next run", row)
			}
		})
	}
}

func TestFundRefund_aMissingTransactionIsSentAgainUntilTheWaitEnds(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.stubs.finalize = false
	h.stubs.nextStatus = &solana.Status{State: solana.StateNotFound}
	h.stubs.sendErr = errDependency
	refunds := h.refundsWith(func(d *app.FundRefundDeps) { d.Clock, d.Wait = clock.Real{}, 20*time.Millisecond })
	id := h.landed(t, "5000000")
	got, err := refunds.Refund(h.systemCtx(), app.RefundFundTransfer{ID: id})
	if err != nil || got.State != app.FundRefundPending || len(h.stubs.broadcasts) < 3 {
		t.Fatalf("Refund = %+v, %v after %d sends, want pending after sending again", got, err, len(h.stubs.broadcasts))
	}
}

func TestFundRefund_aCancelledRunStopsWaitingAndKeepsTheSignature(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.stubs.finalize = false
	refunds := h.refundsWith(func(d *app.FundRefundDeps) { d.Clock, d.Wait = clock.Real{}, time.Hour })
	ctx, cancel := context.WithCancel(h.systemCtx())
	h.stubs.onStatuses = cancel
	id := h.landed(t, "5000000")
	_, err := refunds.Refund(ctx, app.RefundFundTransfer{ID: id})
	wantCode(t, err, errs.CodeUpstreamUnavailable)
	if row := h.row(t, id); row.status != "landed" || row.signature == "" {
		t.Fatalf("row = %+v, want landed with its signature", row)
	}
}

func TestFundRefund_aSendFailureOrAMissingTransferFactoryDoesNotStopTheConfirmation(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.stubs.finalize = false
	id := h.landed(t, "5000000")
	if _, err := h.refund(id); err != nil {
		t.Fatal(err)
	}
	h.finalize(chain.Signature(h.row(t, id).signature))
	h.stubs.factoryErr = errDependency
	if got, err := h.refund(id); err != nil || got.State != app.FundRefundDone {
		t.Fatalf("resume = %+v, %v, want refunded although the factory failed", got, err)
	}
}

func TestFundRefund_aRunThatLosesTheRaceReportsWhatTheOtherRunDid(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	id := h.landed(t, "5000000")
	h.stubs.finalize = true
	h.stubs.onStatuses = func() {
		if _, err := h.pool.Exec(t.Context(), `UPDATE fund_transfers SET status = 'refunded', refunded_at = now()
			WHERE id = $1`, id); err != nil {
			t.Error(err)
		}
	}
	got, err := h.refund(id)
	if err != nil || got.State != app.FundRefundAlready || h.count(t, "user_txns") != 0 {
		t.Fatalf("Refund = %+v, %v with %d ledger txns, want already_refunded and nothing booked", got, err,
			h.count(t, "user_txns"))
	}
}

func TestFundRefund_aDroppedSignatureClearedByAnotherRunIsNotLoggedTwice(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.stubs.finalize = false
	h.stubs.nextStatus = &solana.Status{State: solana.StateFinalized, Failed: true}
	h.stubs.onStatuses = func() {
		if _, err := h.pool.Exec(
			t.Context(),
			`UPDATE fund_transfers SET refund_signed_tx = NULL, refund_signature = NULL,
			refund_last_valid_block_height = NULL, refund_submitted_at = NULL`,
		); err != nil {
			t.Error(err)
		}
	}
	id := h.landed(t, "5000000")
	_, err := h.refund(id)
	wantCode(t, err, errs.CodeFundRefundRejected)
}

func (h *refundHarness) deadPool(t *testing.T) *pgxpool.Pool {
	t.Helper()
	dead, err := db.Open(t.Context(), config.DB{URL: h.pool.Config().ConnString(), MaxConns: 2})
	if err != nil {
		t.Fatal(err)
	}
	dead.Close()
	return dead
}

func (h *refundHarness) deadUoW(t *testing.T) *db.UnitOfWork {
	t.Helper()
	return db.New(h.deadPool(t), h.ids, h.clock)
}

func TestFundRefund_aDatabaseThatFailsMidRunLeavesTheTransferRetryable(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	dead := h.refundsWith(func(d *app.FundRefundDeps) { d.UoW = h.deadUoW(t) })
	id := h.landed(t, "5000000")
	if _, err := dead.Refund(h.systemCtx(), app.RefundFundTransfer{ID: id}); err == nil {
		t.Fatal("Refund stored a signature through a closed database")
	}
	if row := h.row(t, id); row.status != "landed" || row.signature != "" || len(h.stubs.broadcasts) != 0 {
		t.Fatalf("row = %+v, broadcasts %d, want landed with nothing stored or sent", row, len(h.stubs.broadcasts))
	}
	h.stubs.finalize = false
	if _, err := h.refund(id); err != nil {
		t.Fatal(err)
	}
	h.finalize(chain.Signature(h.row(t, id).signature))
	if _, err := dead.Refund(h.systemCtx(), app.RefundFundTransfer{ID: id}); err == nil {
		t.Fatal("Refund booked through a closed database")
	}
	if got := h.row(t, id); got.status != "landed" || got.signature == "" {
		t.Fatalf("row = %+v, want landed with its signature after the failed booking", got)
	}
	h.stubs.nextStatus = &solana.Status{State: solana.StateFinalized, Failed: true}
	if _, err := dead.Refund(h.systemCtx(), app.RefundFundTransfer{ID: id}); err == nil {
		t.Fatal("Refund dropped a signature through a closed database")
	}
}

func TestFundRefund_aRowItCannotReadOrDecodeIsRefused(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	closed := h.deadPool(t)
	unreadable := h.refundsWith(func(d *app.FundRefundDeps) { d.Reads = closed })
	_, err := unreadable.Plan(h.systemCtx(), h.ids.NewV7())
	wantCode(t, err, errs.CodeInternal)
	id := h.landed(t, "99999999999999999999")
	_, err = h.refunds.Plan(h.systemCtx(), id)
	wantCode(t, err, errs.CodeDecodeFailed)
	id = h.landed(t, "5000000")
	if _, err := h.pool.Exec(t.Context(), `UPDATE fund_transfers SET refund_signed_tx = '\x01', refund_signature = 'x',
		refund_last_valid_block_height = -1, refund_submitted_at = now() WHERE id = $1`, id); err != nil {
		t.Fatal(err)
	}
	_, err = h.refunds.Plan(h.systemCtx(), id)
	wantCode(t, err, errs.CodeDecodeFailed)
}

func TestFundRefund_theTreasuryMustHoldExactlyEnoughNotMore(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.stubs.balance = 5_000_000
	id := h.landed(t, "5000000")
	plan, err := h.refunds.Plan(h.systemCtx(), id)
	if err != nil || plan.Treasury != money.MicrosFromUint64(5_000_000) {
		t.Fatalf("Plan = %+v, %v, want the exact balance to cover the refund", plan, err)
	}
}

func TestFundRefund_anAmountTheLedgerCannotHoldIsSentButNotBooked(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.stubs.balance = 18_000_000_000_000_000_000
	id := h.landed(t, "10000000000000000000")
	_, err := h.refund(id)
	if err == nil {
		t.Fatal("Refund booked an amount past the ledger's range")
	}
	if got := h.row(t, id); got.status != "landed" || got.signature == "" || h.count(t, "user_txns") != 0 {
		t.Fatalf("row = %+v, want landed with its signature and nothing booked", got)
	}
}
