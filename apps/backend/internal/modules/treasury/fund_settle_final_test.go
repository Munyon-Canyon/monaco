package treasury_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
)

func TestFundSettler_aProcessedErrorStaysPendingAndAFinalSuccessStillMints(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	id, sig := h.submitted(t, 5_000_000)
	h.chain.statuses[sig] = solana.Status{Signature: sig, State: solana.StateProcessing, Failed: true}
	h.tick(t)
	if status, _, code := h.status(t, id); status != "submitted" || code != "" {
		t.Fatalf("status after a processed error = %s/%s, want submitted with no fail code", status, code)
	}
	h.finalize(sig)
	h.tick(t)
	if status, units, _ := h.status(t, id); status != "settled" || units != "5000000" {
		t.Fatalf("status after the signature finalized = %s/%s, want settled 5000000", status, units)
	}
	h.assertMinted(t, id)
	if n := h.events(t, "cabal.fund_failed"); n != 0 {
		t.Fatalf("cabal.fund_failed = %d, want none", n)
	}
}

func TestFundSettler_aFinalizedErrorStillFailsTheFund(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	id, sig := h.submitted(t, 5_000_000)
	h.chain.statuses[sig] = solana.Status{Signature: sig, State: solana.StateFinalized, Failed: true}
	h.tick(t)
	if status, _, code := h.status(t, id); status != "failed" || code != "fund_rejected" {
		t.Fatalf("status = %s/%s, want failed/fund_rejected", status, code)
	}
}

func TestFundSettler_mintsLandedFundsWhenTheStatusReadFails(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	_, sig := h.submitted(t, 5_000_000)
	h.finalize(sig)
	landed := h.landed(t, "7000000")
	h.chain.statusErr = errs.New(errs.CodeRPCUnavailable, "stub")
	wantCode(t, h.tickErr(), errs.CodeRPCUnavailable)
	if status, units, _ := h.status(t, landed); status != "settled" || units != "7000000" {
		t.Fatalf("landed fund = %s/%s, want settled 7000000 despite the failed status read", status, units)
	}
}

func TestFundSettler_oneRowsWriteErrorDoesNotStopTheOthers(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	bad, badSig := h.submitted(t, 5_000_000)
	good, goodSig := h.submitted(t, 5_000_000)
	if _, err := h.pool.Exec(
		t.Context(), `UPDATE fund_transfers SET amount_micros = 99999999999999999999 WHERE id = $1`, bad,
	); err != nil {
		t.Fatal(err)
	}
	h.chain.statuses[badSig] = solana.Status{Signature: badSig, State: solana.StateFinalized, Failed: true}
	h.finalize(goodSig)
	if err := h.tickErr(); err == nil {
		t.Fatal("Tick over an unparsable amount = nil error")
	}
	if status, units, _ := h.status(t, good); status != "settled" || units != "5000000" {
		t.Fatalf("good fund = %s/%s, want settled 5000000 past the other row's error", status, units)
	}
}
