package treasury_test

import (
	"context"
	"encoding/json"
	"math"
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestWithdrawalLedgerPostsOnceAndBalancesAgainstTheEvents(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := f.user(t)
	h := adapters.UserLedger{Ledger: f.ledger, IDs: f.ids, USDC: usdc, Hints: &hints{}}
	deposit := events.DepositCredited{
		V: 1, DepositID: f.ids.NewV7(), UserID: user.UUID(), TxSignature: "deposit",
		AmountMicros: money.MicrosFromUint64(25),
	}
	withdrawal := events.WithdrawalConfirmed{
		V: 1, WithdrawalID: f.ids.NewV7(), UserID: user.UUID(), AmountMicros: money.MicrosFromUint64(10),
		ToAddress: chain.SolanaAddress("9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P"), TxSignature: "withdrawal",
	}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		ctx = observability.WithActor(ctx, "system:test")
		if err := tx.Events.Append(ctx, deposit); err != nil {
			return err
		}
		if err := tx.Events.Append(ctx, withdrawal); err != nil {
			return err
		}
		return h.Handle(ctx, tx, deposit, f.clock.Now())
	}); err != nil {
		t.Fatal(err)
	}
	hints := &hints{}
	h.Hints = hints
	for range 2 {
		if err := f.do(func(ctx context.Context, tx db.Tx) error {
			return h.Withdrawal(ctx, tx, withdrawal, f.clock.Now())
		}); err != nil {
			t.Fatal(err)
		}
	}
	assertWithdrawalRow(t, f, user)
	if !slices.Contains(hints.keys, events.UserBalanceChangedHint(user)) {
		t.Fatalf("hints = %v, want the user's balance_changed", hints.keys)
	}
	if diffs, err := treasury.LedgerCheck(testkit.Config()).Check(t.Context(), f.pool); err != nil || len(diffs) != 0 {
		t.Fatalf("ledger check = %q, %v, want no diffs", diffs, err)
	}
}

func assertWithdrawalRow(t *testing.T, f fixture, user ids.UserID) {
	t.Helper()
	var kind, status, signature, wallet, external string
	if err := f.pool.QueryRow(t.Context(), `SELECT t.kind, t.status, t.tx_signature,
		(SELECT amount::text FROM user_txn_entries WHERE txn_id = t.id AND account = 'wallet'),
		(SELECT amount::text FROM user_txn_entries WHERE txn_id = t.id AND account = 'external')
		FROM user_txns t WHERE t.user_id = $1 AND t.kind = 'withdrawal'`, user.UUID(),
	).Scan(&kind, &status, &signature, &wallet, &external); err != nil {
		t.Fatal(err)
	}
	if status != "settled" || signature != "withdrawal" || wallet != "-10" || external != "10" {
		t.Fatalf("withdrawal txn = %s %s %s wallet %s external %s", kind, status, signature, wallet, external)
	}
}

func TestWithdrawalLedgerRejectsUnrepresentableAmounts(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	h := adapters.UserLedger{Ledger: f.ledger, IDs: f.ids, USDC: usdc, Hints: &hints{}}
	for name, e := range map[string]events.WithdrawalConfirmed{
		"too large": {AmountMicros: money.MicrosFromUint64(math.MaxUint64)},
		"zero":      {AmountMicros: money.Micros{}},
	} {
		if err := f.do(func(ctx context.Context, tx db.Tx) error {
			return h.Withdrawal(ctx, tx, e, f.clock.Now())
		}); err == nil {
			t.Fatalf("%s withdrawal error = nil", name)
		}
	}
	valid := events.WithdrawalConfirmed{
		V: 1, WithdrawalID: f.ids.NewV7(), UserID: f.user(t).UUID(), AmountMicros: money.MicrosFromUint64(10),
	}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		dead, cancel := context.WithCancel(ctx)
		cancel()
		return h.Withdrawal(dead, tx, valid, f.clock.Now())
	}); err == nil {
		t.Fatal("withdrawal on a dead context error = nil")
	}
	rule := adapters.WithdrawalConfirmedBalances(usdc)
	payload, err := json.Marshal(events.WithdrawalConfirmed{AmountMicros: money.MicrosFromUint64(math.MaxUint64)})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := rule(payload); err == nil {
		t.Fatal("unrepresentable balance error = nil")
	}
	if _, err := rule([]byte("{")); err == nil {
		t.Fatal("malformed withdrawal event error = nil")
	}
}
