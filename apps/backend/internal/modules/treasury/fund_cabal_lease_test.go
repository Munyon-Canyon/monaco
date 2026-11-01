package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
)

func TestFundCabal_aRetryWhileTheFirstRequestIsSigningReturnsItsTransferWithoutSigning(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	var retryID uuid.UUID
	var retryErr error
	buildsInRetry := -1
	h.stubs.onBuild = func() {
		h.stubs.onBuild = nil
		retryID, retryErr = h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
		buildsInRetry = h.stubs.builds
	}
	firstID, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	if retryErr != nil || retryID != firstID || buildsInRetry != 1 {
		t.Fatalf("retry = (%v, %v) with %d builds, want the first transfer %v, no error and no second sign",
			retryID, retryErr, buildsInRetry, firstID)
	}
	if got := h.transfer(t, firstID); got.Status != string(domain.FundSubmitted) || got.FailCode != "" ||
		h.transferRows(t) != 1 || h.stubs.sent != 1 || h.stubs.builds != 1 {
		t.Fatalf("row = %s/%q rows = %d sent = %d builds = %d, want one submitted row signed and sent once",
			got.Status, got.FailCode, h.transferRows(t), h.stubs.sent, h.stubs.builds)
	}
}

func TestFundCabal_aRequestThatLosesTheSubmitToAnAlreadySentRowReturnsItsIdAndBroadcastsNothing(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	var retryErr error
	h.stubs.onBuild = func() {
		h.stubs.onBuild = nil
		h.clock.Advance(fundSendWindow + time.Second)
		_, retryErr = h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
		h.stubs.mu.Lock()
		defer h.stubs.mu.Unlock()
		if h.stubs.sent != 1 {
			t.Errorf("broadcasts after the retry = %d, want 1", h.stubs.sent)
		}
	}
	first, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	if err != nil || retryErr != nil {
		t.Fatalf("first = (%v, %v), retry = %v, want neither to fail", first, err, retryErr)
	}
	if got := h.transfer(t, first); got.Status != string(domain.FundSubmitted) || h.transferRows(t) != 1 ||
		h.stubs.sent != 1 || h.events(t, "cabal.fund_submitted") != 1 || h.stubs.builds != 2 {
		t.Fatalf("row = %s rows = %d sent = %d events = %d builds = %d, want one row sent once by the retry",
			got.Status, h.transferRows(t), h.stubs.sent, h.events(t, "cabal.fund_submitted"), h.stubs.builds)
	}
}

func TestFundCabal_aReplayInsideTheLeaseAfterACrashReturnsTheIdAndLeavesTheRowCreated(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	h.stubs.onBuild = func() { panic("killed while signing") }
	func() {
		defer func() { _ = recover() }()
		_, _ = h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	}()
	h.stubs.onBuild = nil
	h.clock.Advance(fundSendWindow - time.Second)
	id, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	if err != nil || h.stubs.builds != 1 || h.stubs.sent != 0 ||
		h.transfer(t, id).Status != string(domain.FundCreated) {
		t.Fatalf("replay = (%v, %v) builds = %d sent = %d, want the id, no new sign and a created row",
			id, err, h.stubs.builds, h.stubs.sent)
	}
}

func TestFundCabal_aClaimThatCannotBeStoredFailsTheReplayWithoutSigning(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	h.stubs.onBuild = func() { panic("killed while signing") }
	func() {
		defer func() { _ = recover() }()
		_, _ = h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	}()
	h.stubs.onBuild = nil
	if _, err := h.pool.Exec(context.Background(), `
CREATE FUNCTION refuse_lease() RETURNS trigger LANGUAGE plpgsql AS $$ BEGIN RAISE EXCEPTION 'no lease'; END $$;
CREATE TRIGGER refuse_lease BEFORE UPDATE OF signing_until ON fund_transfers
FOR EACH ROW EXECUTE FUNCTION refuse_lease()`); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(fundSendWindow + time.Second)
	_, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	if err == nil || h.stubs.builds != 1 || h.stubs.sent != 0 {
		t.Fatalf("replay = %v builds = %d sent = %d, want an error and no new sign", err, h.stubs.builds, h.stubs.sent)
	}
}

func TestFundCabal_aSubmitThatCannotReadTheRowAfterLosingReturnsTheError(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	h.stubs.onBuild = func() {
		if _, err := h.pool.Exec(context.Background(), `DELETE FROM fund_transfers`); err != nil {
			t.Error(err)
		}
	}
	_, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	if err == nil || errs.CodeOf(err) == errs.CodeFundNotSent || h.stubs.sent != 0 {
		t.Fatalf("fund = %v sent = %d, want the read error and no broadcast", err, h.stubs.sent)
	}
}
