package treasury_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestFundActivity_tracksATransferFromPendingToConfirmed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal, transfer, other := f.user(t), f.cabal(t), f.ids.NewV7(), f.ids.NewV7()
	sig := chain.Signature("fund-signature")
	submitted := events.FundSubmitted{
		V: 1, TransferID: transfer, CabalID: cabal.UUID(), UserID: user.UUID(),
		AmountMicros: money.MicrosFromUint64(5_000_000), TxSignature: sig,
	}
	funded := events.Funded{
		V: 1, TransferID: transfer, CabalID: cabal.UUID(), UserID: user.UUID(),
		AmountMicros: money.MicrosFromUint64(5_000_000), ShareUnits: money.SharesUnitsFromUint64(4_000_000),
		TxSignature: sig,
	}
	failed := events.FundFailed{
		V: 1, TransferID: other, CabalID: cabal.UUID(), UserID: user.UUID(),
		AmountMicros: money.MicrosFromUint64(2_000_000), Code: "fund_rejected",
	}
	for _, e := range []events.Event{submitted, funded, submitted, failed} {
		if err := f.deliver(t, e, submittedAt()); err != nil {
			t.Fatal(err)
		}
	}
	rows := f.activity(t)
	if len(rows) != 2 {
		t.Fatalf("activity rows = %d, want 2", len(rows))
	}
	byID := map[string]activityRow{rows[0].ID.String(): rows[0], rows[1].ID.String(): rows[1]}
	wantActivity(t, byID[transfer.String()], "fund", "confirmed", nil, str("5000000"), str("4000000"), str(string(sig)))
	wantActivity(t, byID[other.String()], "fund", "failed", nil, str("2000000"), nil, nil)
	var actors int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM cabal_activity WHERE actor_user_id = $1`,
		user.UUID()).Scan(&actors); err != nil || actors != 2 {
		t.Fatalf("activity rows with the funder as actor = %d, %v, want 2", actors, err)
	}
}

type namedCabals struct{}

func (namedCabals) Cabals(_ context.Context, list []ids.CabalID) (map[ids.CabalID]cabalport.CabalView, error) {
	out := map[ids.CabalID]cabalport.CabalView{}
	for _, id := range list {
		out[id] = cabalport.CabalView{ID: id, Name: "Friends"}
	}
	return out, nil
}

func TestUserTxns_listOpenFundsAsPendingOrFailed(t *testing.T) {
	t.Parallel()
	h := newSettleHarness(t)
	pending, err := h.fund(5_000_000)
	if err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Second)
	settledID, settledSig := h.submitted(t, 7_000_000)
	h.finalize(settledSig)
	h.tick(t)
	h.clock.Advance(time.Second)
	h.stubs.buildErr = errs.New(errs.CodePrivyUnavailable, "stub")
	_, _ = h.fund(3_000_000)
	reads := app.NewUserTxnReads(h.pool, namedCabals{}, noWithdrawals{}, h.usdc())
	page, err := reads.List(h.ctx(), app.ListUserTxns{UserID: h.user})
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, item := range page.Items {
		if item.Kind != "fund" || item.Cabal == nil || item.Cabal.Name != "Friends" {
			t.Fatalf("item = %+v, want a fund in Friends", item)
		}
		got[string(item.Status)] += money.SignedMicrosFromInt64(item.USDCMicros).String() + " "
	}
	want := map[string]string{"pending": "-5000000 ", "settled": "-7000000 ", "failed": "-3000000 "}
	for status, amount := range want {
		if got[status] != amount {
			t.Fatalf("txns = %v, want %v (pending %s, settled %s)", got, want, pending, settledID)
		}
	}
}
