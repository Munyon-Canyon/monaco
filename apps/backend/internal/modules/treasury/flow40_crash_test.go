//go:build faultpoints

package treasury_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (h *refundHarness) crashRefund(t *testing.T, point faultpoint.Name) {
	t.Helper()
	id := h.landed(t, "5000000")
	testkit.CrashAt(t, point, func(ctx context.Context) error {
		_, err := h.refunds.Refund(
			withSystemActor(ctx),
			app.RefundFundTransfer{ID: id, Actor: "operator:test", Reason: "stuck"},
		)
		return err
	})
	row := h.row(t, id)
	if row.status != "refunded" || !row.refundedAt || h.events(t, "cabal.fund_refunded") != 1 ||
		h.count(t, "user_txns") != 1 {
		t.Fatalf("after a crash at %s: %+v with %d events and %d ledger txns, want one refund", point, row,
			h.events(t, "cabal.fund_refunded"), h.count(t, "user_txns"))
	}
	if drift := h.drift(t); len(drift) != 0 {
		t.Fatalf("ledger drift = %v", drift)
	}
}

func TestFlow40_RefundFundTransfer_CrashAfterSign(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.crashRefund(t, faultpoint.AfterSign)
	if len(h.stubs.built) != 2 || len(h.stubs.broadcasts) != 1 {
		t.Fatalf(
			"built %d, broadcast %d, want a second signing and one send",
			len(h.stubs.built),
			len(h.stubs.broadcasts),
		)
	}
}

func TestFlow40_RefundFundTransfer_CrashAfterBroadcast(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.crashRefund(t, faultpoint.AfterBroadcast)
	if len(h.stubs.built) != 1 || len(h.stubs.broadcasts) != 2 ||
		h.stubs.broadcasts[0].Signature != h.stubs.broadcasts[1].Signature {
		t.Fatalf("built %d, broadcast %d, want one signing and the same transaction sent again",
			len(h.stubs.built), len(h.stubs.broadcasts))
	}
}

func TestFlow40_RefundFundTransfer_CrashBeforeCommit(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	h.crashRefund(t, faultpoint.BeforeCommit)
	if len(h.stubs.built) != 2 || len(h.stubs.broadcasts) != 1 {
		t.Fatalf(
			"built %d, broadcast %d, want a second signing and one send",
			len(h.stubs.built),
			len(h.stubs.broadcasts),
		)
	}
}

func TestFundRefund_crashBeforeTheBookingCommitKeepsTheStoredSignature(t *testing.T) {
	t.Parallel()
	h := newRefundHarness(t)
	id := h.landed(t, "5000000")
	cmd := app.RefundFundTransfer{ID: id, Actor: "operator:test", Reason: "stuck"}
	crashed := func() (p any) {
		defer func() { p = recover() }()
		_, _ = h.refunds.Refund(faultpoint.ArmedAfter(withSystemActor(t.Context()), faultpoint.BeforeCommit, 1), cmd)
		return nil
	}()
	if crashed != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
		t.Fatalf("Refund did not crash before the booking commit: %v", crashed)
	}
	if row := h.row(t, id); row.status != "landed" || row.signature == "" || h.count(t, "user_txns") != 0 {
		t.Fatalf("after the crash: %+v with %d ledger txns, want landed with its signature and nothing booked",
			row, h.count(t, "user_txns"))
	}
	if _, err := h.refunds.Refund(withSystemActor(t.Context()), cmd); err != nil {
		t.Fatal(err)
	}
	if h.row(t, id).status != "refunded" || len(h.stubs.built) != 1 || h.events(t, "cabal.fund_refunded") != 1 ||
		h.count(t, "user_txns") != 1 {
		t.Fatalf("after the retry: %+v, built %d, want one signing and one booking", h.row(t, id), len(h.stubs.built))
	}
}
