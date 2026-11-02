package treasury_test

import (
	"context"
	"slices"
	"sync"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func submittedAt() time.Time { return time.Date(2026, 3, 1, 12, 0, 2, 0, time.UTC) }

type hints struct {
	mu   sync.Mutex
	keys []string
}

func (h *hints) PublishHint(_ context.Context, key string, _ []byte) {
	h.mu.Lock()
	defer h.mu.Unlock()
	h.keys = append(h.keys, key)
}

func (h *hints) sent() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return slices.Clone(h.keys)
}

func (f fixture) deliver(t *testing.T, e events.Event, at time.Time) error {
	t.Helper()
	m := treasury.New(module.Deps{Config: f.cfg, Pool: f.pool, IDs: f.ids, Clock: f.clock})
	return f.do(func(ctx context.Context, tx db.Tx) error {
		for _, c := range m.Consumers() {
			for _, h := range c.Handlers {
				if h.Type() != e.Type() {
					continue
				}
				if err := h.Apply(ctx, tx, e, at); err != nil {
					return err
				}
			}
		}
		return nil
	})
}

func submitted(
	cabal ids.CabalID,
	swap uuid.UUID,
	action string,
	in, out chain.SolanaAddress,
	amount uint64,
) events.TradeSubmitted {
	return events.TradeSubmitted{
		V: 1, SwapID: swap, CabalID: cabal.UUID(), Source: events.TradeSource{Kind: "proposal", ID: swap},
		Action: action, Symbol: "AAPLx", InMint: in, OutMint: out, InAmount: amount, TxSignature: swapSig,
	}
}

func failed(
	cabal ids.CabalID,
	swap uuid.UUID,
	action string,
	in chain.SolanaAddress,
	amount uint64,
) events.TradeFailed {
	return events.TradeFailed{
		V: 1, SwapID: swap, CabalID: cabal.UUID(), Source: events.TradeSource{Kind: "proposal", ID: swap},
		Action: action, Symbol: "AAPLx", InMint: in, InAmount: amount, FailureCode: "slippage_exceeded",
	}
}

type activityRow struct {
	ID                            uuid.UUID
	Kind, Status                  string
	Asset, USDC, Units, Signature *string
	OccurredAt, UpdatedAt         time.Time
}

func (f fixture) activity(t *testing.T) []activityRow {
	t.Helper()
	rows, err := f.pool.Query(t.Context(), `SELECT id, kind, status, asset, usdc_micros::text, units::text,
		tx_signature, occurred_at, updated_at FROM cabal_activity ORDER BY occurred_at, id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []activityRow
	for rows.Next() {
		var r activityRow
		if err := rows.Scan(&r.ID, &r.Kind, &r.Status, &r.Asset, &r.USDC, &r.Units, &r.Signature, &r.OccurredAt,
			&r.UpdatedAt); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rows.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func str(s string) *string { return &s }

func wantActivity(t *testing.T, got activityRow, kind, status string, asset, usdc, units, sig *string) {
	t.Helper()
	if got.Kind != kind || got.Status != status || !same(got.Asset, asset) || !same(got.USDC, usdc) ||
		!same(got.Units, units) || !same(got.Signature, sig) {
		t.Fatalf("activity = %s %s asset %v usdc %v units %v sig %v, want %s %s asset %v usdc %v units %v sig %v",
			got.Kind, got.Status, deref(got.Asset), deref(got.USDC), deref(got.Units), deref(got.Signature),
			kind, status, deref(asset), deref(usdc), deref(units), deref(sig))
	}
}

func same(a, b *string) bool { return (a == nil && b == nil) || (a != nil && b != nil && *a == *b) }

func deref(s *string) string {
	if s == nil {
		return "<nil>"
	}
	return *s
}

func TestActivityConsumer_aBuyGoesFromPendingToConfirmedWithItsFill(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	swap := f.ids.NewV7()
	if err := f.deliver(t, submitted(cabal, swap, "buy", usdcMint, chain.SolanaAddress(aapl), 60_000_000),
		submittedAt()); err != nil {
		t.Fatal(err)
	}
	got := f.activity(t)
	if len(got) != 1 {
		t.Fatalf("activity = %+v, want one row", got)
	}
	wantActivity(t, got[0], "buy", "pending", str(string(aapl)), str("60000000"), nil, str(string(swapSig)))
	if err := f.deliver(t, buy(cabal, swap, 60_000_000, 3_000_000, 0), confirmedAt()); err != nil {
		t.Fatal(err)
	}
	got = f.activity(t)
	wantActivity(t, got[0], "buy", "confirmed", str(string(aapl)), str("60000000"), str("3000000"),
		str(string(swapSig)))
	if got[0].ID != swap || !got[0].OccurredAt.Equal(submittedAt()) || !got[0].UpdatedAt.Equal(confirmedAt()) {
		t.Fatalf("activity = %+v, want id %s, occurred at the submit and updated at the confirm", got[0], swap)
	}
	if n := len(f.swapHeaders(t)); n != 1 {
		t.Fatalf("swap headers = %d, want 1", n)
	}
}

func TestActivityConsumer_aSubmitAfterTheConfirmLeavesTheRowConfirmed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	swap := f.ids.NewV7()
	if err := f.deliver(t, buy(cabal, swap, 60_000_000, 3_000_000, 0), confirmedAt()); err != nil {
		t.Fatal(err)
	}
	if err := f.deliver(t, submitted(cabal, swap, "buy", usdcMint, chain.SolanaAddress(aapl), 60_000_000),
		submittedAt()); err != nil {
		t.Fatal(err)
	}
	got := f.activity(t)
	if len(got) != 1 || !got[0].OccurredAt.Equal(confirmedAt()) {
		t.Fatalf("activity = %+v, want one row first seen at the confirm", got)
	}
	wantActivity(t, got[0], "buy", "confirmed", str(string(aapl)), str("60000000"), str("3000000"),
		str(string(swapSig)))
}

func TestActivityConsumer_aFailedSwapHasNoLedgerHeaderAndOneFailedRow(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	sold, bought := f.ids.NewV7(), f.ids.NewV7()
	for _, e := range []events.Event{
		submitted(cabal, sold, "sell", chain.SolanaAddress(aapl), usdcMint, 1_000_000),
		failed(cabal, sold, "sell", chain.SolanaAddress(aapl), 1_000_000),
		failed(cabal, bought, "buy", usdcMint, 5_000_000),
		failed(cabal, bought, "buy", usdcMint, 5_000_000),
	} {
		if err := f.deliver(t, e, submittedAt()); err != nil {
			t.Fatal(err)
		}
	}
	got := f.activity(t)
	if len(got) != 2 || f.count(t, "cabal_txns WHERE kind = 'swap'") != 0 {
		t.Fatalf("activity = %+v and %d swap headers, want two failed rows and none", got,
			f.count(t, "cabal_txns WHERE kind = 'swap'"))
	}
	byID := map[uuid.UUID]activityRow{got[0].ID: got[0], got[1].ID: got[1]}
	wantActivity(t, byID[sold], "sell", "failed", str(string(aapl)), nil, str("1000000"), str(string(swapSig)))
	wantActivity(t, byID[bought], "buy", "failed", nil, str("5000000"), nil, nil)
}

func TestActivityConsumer_aCancelledSourceWritesNoTradeFailedRow(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	swap := f.ids.NewV7()
	cancelled := failed(cabal, swap, "buy", usdcMint, 5_000_000)
	cancelled.FailureCode = "source_cancelled"
	if err := f.deliver(t, cancelled, submittedAt()); err != nil {
		t.Fatal(err)
	}
	if got := f.activity(t); len(got) != 0 {
		t.Fatalf("activity = %+v, want no row for a swap whose proposal was voided", got)
	}
}

func TestActivityConsumer_hintsTheCabalOncePerChangeAndKeepsCashOutSellsOutOfTheHints(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sent := &hints{}
	h := adapters.Activity{Hints: sent}
	cabal := f.cabal(t)
	swap, cashout := f.ids.NewV7(), f.ids.NewV7()
	sale := submitted(cabal, cashout, "sell", chain.SolanaAddress(aapl), usdcMint, 1)
	sale.Source.Kind = "cashout"
	steps := []func(ctx context.Context, tx db.Tx) error{
		func(ctx context.Context, tx db.Tx) error {
			return h.Submitted(
				ctx,
				tx,
				submitted(cabal, swap, "buy", usdcMint, chain.SolanaAddress(aapl), 9),
				submittedAt(),
			)
		},
		func(ctx context.Context, tx db.Tx) error {
			return h.Submitted(
				ctx,
				tx,
				submitted(cabal, swap, "buy", usdcMint, chain.SolanaAddress(aapl), 9),
				submittedAt(),
			)
		},
		func(ctx context.Context, tx db.Tx) error {
			return h.Failed(ctx, tx, failed(cabal, swap, "buy", usdcMint, 9), confirmedAt())
		},
		func(ctx context.Context, tx db.Tx) error { return h.Submitted(ctx, tx, sale, submittedAt()) },
	}
	for _, step := range steps {
		if err := f.do(step); err != nil {
			t.Fatal(err)
		}
	}
	key := events.CabalActivityChangedHint(cabal)
	if got := sent.sent(); !slices.Equal(got, []string{key, key}) {
		t.Fatalf("hints = %q, want %q twice: the insert and the move to failed", got, key)
	}
	got := f.activity(t)
	if len(got) != 2 || got[0].ID != swap || got[1].ID != cashout {
		t.Fatalf("activity = %+v, want the proposal swap and the cash-out sale", got)
	}
	var flagged []uuid.UUID
	rows, err := f.pool.Query(t.Context(), `SELECT id FROM cabal_activity WHERE cashout_sale`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		flagged = append(flagged, id)
	}
	if !slices.Equal(flagged, []uuid.UUID{cashout}) {
		t.Fatalf("cash-out sales = %v, want only %v", flagged, cashout)
	}
}

func TestActivityConsumer_refusesAnUnknownAction(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	err := f.deliver(t, failed(f.cabal(t), f.ids.NewV7(), "hold", usdcMint, 1), submittedAt())
	wantCode(t, err, errs.CodeInvalidInput)
	if n := f.count(t, "cabal_activity"); n != 0 {
		t.Fatalf("cabal_activity = %d rows, want 0", n)
	}
}

func TestActivityConsumer_aSubmitAfterTheFailureFillsTheAssetButStaysFailed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal, swap := f.cabal(t), f.ids.NewV7()
	for _, e := range []events.Event{
		failed(cabal, swap, "buy", usdcMint, 5_000_000),
		submitted(cabal, swap, "buy", usdcMint, chain.SolanaAddress(aapl), 5_000_000),
	} {
		if err := f.deliver(t, e, submittedAt()); err != nil {
			t.Fatal(err)
		}
	}
	got := f.activity(t)
	if len(got) != 1 {
		t.Fatalf("activity = %+v, want one row", got)
	}
	wantActivity(t, got[0], "buy", "failed", str(string(aapl)), str("5000000"), nil, str(string(swapSig)))
}

func cashOutStarted(cabal ids.CabalID, user ids.UserID, job uuid.UUID, payout uint64) events.CashOutStarted {
	return events.CashOutStarted{
		V: 1, JobID: job, CabalID: cabal.UUID(), UserID: user.UUID(),
		ShareUnits: 40_000_000, PayoutMicros: money.MicrosFromUint64(payout),
	}
}

func cashOutCompleted(cabal ids.CabalID, user ids.UserID, job uuid.UUID, payout uint64) events.CashOutCompleted {
	return events.CashOutCompleted{
		V: 1, JobID: job, CabalID: cabal.UUID(), UserID: user.UUID(),
		ShareUnits: 40_000_000, PayoutMicros: money.MicrosFromUint64(payout), Signature: swapSig,
	}
}

func cashOutPartial(cabal ids.CabalID, user ids.UserID, job uuid.UUID, payout uint64) events.CashOutPartial {
	return events.CashOutPartial{
		V: 1, JobID: job, CabalID: cabal.UUID(), UserID: user.UUID(), ShareUnitsBurned: 24_000_000,
		ShareUnitsReturned: 16_000_000, PayoutMicros: money.MicrosFromUint64(payout), Signature: swapSig,
	}
}

func cashOutFailed(cabal ids.CabalID, user ids.UserID, job uuid.UUID) events.CashOutFailed {
	return events.CashOutFailed{
		V: 1, JobID: job, CabalID: cabal.UUID(), UserID: user.UUID(), ShareUnits: 40_000_000, Code: "payout_failed",
	}
}

func (f fixture) cashOutActors(t *testing.T, user ids.UserID) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM cabal_activity WHERE kind = 'cash_out'
		AND actor_user_id = $1`, user.UUID()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestActivityConsumer_recordsACashOutFromPendingToConfirmed(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	sent := &hints{}
	h := adapters.Activity{Hints: sent}
	cabal, user, job := f.cabal(t), f.user(t), f.ids.NewV7()
	steps := []func(ctx context.Context, tx db.Tx) error{
		func(ctx context.Context, tx db.Tx) error {
			return h.CashOutStarted(ctx, tx, cashOutStarted(cabal, user, job, 50_000_000), submittedAt())
		},
		func(ctx context.Context, tx db.Tx) error {
			return h.CashOutCompleted(ctx, tx, cashOutCompleted(cabal, user, job, 50_000_000), confirmedAt())
		},
	}
	for _, step := range steps {
		if err := f.do(step); err != nil {
			t.Fatal(err)
		}
	}
	rows := f.activity(t)
	if len(rows) != 1 || rows[0].ID != job {
		t.Fatalf("activity = %+v, want one row for the cash out", rows)
	}
	wantActivity(t, rows[0], "cash_out", "confirmed", nil, str("50000000"), nil, str(string(swapSig)))
	if n := f.cashOutActors(t, user); n != 1 {
		t.Fatalf("cash outs with the member as actor = %d, want 1", n)
	}
	key := events.CabalActivityChangedHint(cabal)
	if got := sent.sent(); !slices.Equal(got, []string{key, key}) {
		t.Fatalf("hints = %q, want %q twice: the insert and the move to confirmed", got, key)
	}
}

func TestActivityConsumer_recordsAPartialCashOutAtTheAmountPaid(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	h := adapters.Activity{Hints: &hints{}}
	cabal, user, job := f.cabal(t), f.user(t), f.ids.NewV7()
	steps := []func(ctx context.Context, tx db.Tx) error{
		func(ctx context.Context, tx db.Tx) error {
			return h.CashOutStarted(ctx, tx, cashOutStarted(cabal, user, job, 50_000_000), submittedAt())
		},
		func(ctx context.Context, tx db.Tx) error {
			return h.CashOutPartial(ctx, tx, cashOutPartial(cabal, user, job, 30_000_000), confirmedAt())
		},
	}
	for _, step := range steps {
		if err := f.do(step); err != nil {
			t.Fatal(err)
		}
	}
	rows := f.activity(t)
	if len(rows) != 1 {
		t.Fatalf("activity = %+v, want one row", rows)
	}
	wantActivity(t, rows[0], "cash_out", "confirmed", nil, str("30000000"), nil, str(string(swapSig)))
}

func TestActivityConsumer_recordsAFailedCashOutWithItsStartAmount(t *testing.T) {
	t.Parallel()
	for name, startFirst := range map[string]bool{"started first": true, "failed first": false} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			h := adapters.Activity{Hints: &hints{}}
			cabal, user, job := f.cabal(t), f.user(t), f.ids.NewV7()
			start := func(ctx context.Context, tx db.Tx) error {
				return h.CashOutStarted(ctx, tx, cashOutStarted(cabal, user, job, 50_000_000), submittedAt())
			}
			fail := func(ctx context.Context, tx db.Tx) error {
				return h.CashOutFailed(ctx, tx, cashOutFailed(cabal, user, job), confirmedAt())
			}
			steps := []func(ctx context.Context, tx db.Tx) error{start, fail}
			if !startFirst {
				steps = []func(ctx context.Context, tx db.Tx) error{fail, start}
			}
			for _, step := range steps {
				if err := f.do(step); err != nil {
					t.Fatal(err)
				}
			}
			rows := f.activity(t)
			if len(rows) != 1 {
				t.Fatalf("activity = %+v, want one row", rows)
			}
			wantActivity(t, rows[0], "cash_out", "failed", nil, str("50000000"), nil, nil)
		})
	}
}
