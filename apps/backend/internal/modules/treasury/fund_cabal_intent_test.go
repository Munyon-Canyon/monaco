package treasury_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (h *fundHarness) fundKeyed(ctx context.Context, hash string, micros uint64) (uuid.UUID, error) {
	return h.handler.Handle(ctx, app.FundCabal{
		CabalID: h.cabal, UserID: h.user, Amount: money.MicrosFromUint64(micros),
		Intent: db.IntentKey{
			ActorKey: "user:" + h.user.String(), Key: "k1", Operation: app.FundCabalOperation,
			RequestHash: []byte(hash),
		},
	})
}

func (h *fundHarness) transferRows(t *testing.T) int {
	t.Helper()
	return h.count(t, "fund_transfers")
}

func TestFundCabal_aReplayWithTheSameKeyReturnsTheFirstTransferAndSendsOnce(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	first, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	again, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	if err != nil || again != first {
		t.Fatalf("replay = (%v, %v), want the first transfer %v", again, err, first)
	}
	if h.transferRows(t) != 1 || h.stubs.sent != 1 || h.events(t, "cabal.fund_submitted") != 1 {
		t.Fatalf("rows = %d sent = %d events = %d, want one of each",
			h.transferRows(t), h.stubs.sent, h.events(t, "cabal.fund_submitted"))
	}
}

func TestFundCabal_aReplayDoesNotNeedTheBalanceReadToSucceed(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	first, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	h.stubs.failOn = "Available"
	again, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	if err != nil || again != first || h.transferRows(t) != 1 {
		t.Fatalf("replay with the balance read down = (%v, %v) rows = %d, want the first transfer %v",
			again, err, h.transferRows(t), first)
	}
}

func TestFundCabal_aReplayAfterACrashBeforeTheSignedBytesResumesTheSameRow(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	h.stubs.onBuild = func() { panic("killed while signing") }
	func() {
		defer func() { _ = recover() }()
		_, _ = h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	}()
	h.stubs.onBuild = nil
	if h.transferRows(t) != 1 || h.stubs.sent != 0 {
		t.Fatalf("after the crash rows = %d sent = %d, want 1 and 0", h.transferRows(t), h.stubs.sent)
	}
	id, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	got := h.transfer(t, id)
	if got.Status != string(domain.FundSubmitted) || h.transferRows(t) != 1 || h.stubs.sent != 1 {
		t.Fatalf("resume = %+v rows = %d sent = %d, want one submitted row sent once",
			got, h.transferRows(t), h.stubs.sent)
	}
}

func TestFundCabal_theSameKeyWithAnotherRequestIsAMismatch(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	if _, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000); err != nil {
		t.Fatal(err)
	}
	_, err := h.fundKeyed(h.actorCtx(), "h2", 6_000_000)
	wantCode(t, err, errs.CodeIdempotencyMismatch)
	if h.transferRows(t) != 1 || h.stubs.sent != 1 {
		t.Fatalf("rows = %d sent = %d, want 1 and 1", h.transferRows(t), h.stubs.sent)
	}
}

func TestFundCabal_aFailureBeforeAnythingWasSentFreesTheKey(t *testing.T) {
	t.Parallel()
	h := newFundHarness(t)
	h.stubs.buildErr = errs.New(errs.CodePrivyUnavailable, "stub")
	_, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	wantCode(t, err, errs.CodePrivyUnavailable)
	if h.count(t, "money_intents") != 0 {
		t.Fatalf("money_intents = %d after an unsent failure, want the key freed", h.count(t, "money_intents"))
	}
	h.stubs.buildErr = nil
	id, err := h.fundKeyed(h.actorCtx(), "h1", 5_000_000)
	if err != nil || h.transfer(t, id).Status != string(domain.FundSubmitted) || h.transferRows(t) != 2 {
		t.Fatalf("retry = (%v, %v) rows = %d, want a new submitted row beside the failed one",
			id, err, h.transferRows(t))
	}
}
