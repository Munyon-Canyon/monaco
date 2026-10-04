package treasury_test

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"slices"
	"sync"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestUserLedgerPostsDepositAndExplainsItsBalance(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := f.user(t)
	e := events.DepositCredited{
		V: 1, DepositID: f.ids.NewV7(), UserID: user.UUID(),
		TxSignature: chain.Signature("deposit"), AmountMicros: money.MicrosFromUint64(25),
	}
	h := adapters.UserLedger{Ledger: f.ledger, IDs: f.ids, USDC: usdc, Hints: &hints{}}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		return h.Handle(ctx, tx, e, f.clock.Now())
	}); err != nil {
		t.Fatal(err)
	}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		return h.Handle(ctx, tx, e, f.clock.Now())
	}); err != nil {
		t.Fatal(err)
	}
	var got string
	if err := f.pool.QueryRow(t.Context(), `SELECT amount::text FROM user_txn_entries
		WHERE account = 'wallet' AND asset = $1`, usdc).Scan(&got); err != nil || got != "25" {
		t.Fatalf("wallet deposit = %q, %v; want 25", got, err)
	}
	payload, err := json.Marshal(e)
	if err != nil {
		t.Fatal(err)
	}
	balances, err := adapters.DepositCreditedBalances(usdc)(payload)
	if err != nil || len(balances) != 1 || balances[0].Amount != money.SignedMicrosFromInt64(25) {
		t.Fatalf("deposit balances = %+v, %v", balances, err)
	}
	if _, err := adapters.DepositCreditedBalances(usdc)([]byte("{")); err == nil {
		t.Fatal("malformed deposit event error = nil")
	}
}

func TestUserLedgerCreditsOneDepositOnceWithTwoRunners(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := f.user(t)
	e := events.DepositCredited{
		V:            1,
		DepositID:    f.ids.NewV7(),
		UserID:       user.UUID(),
		TxSignature:  "deposit",
		AmountMicros: money.MicrosFromUint64(25),
	}
	h := adapters.UserLedger{Ledger: f.ledger, IDs: f.ids, USDC: usdc, Hints: &hints{}}
	start := make(chan struct{})
	errs := make(chan error, 2)
	var wg sync.WaitGroup
	for range 2 {
		wg.Go(func() {
			<-start
			errs <- f.do(func(ctx context.Context, tx db.Tx) error { return h.Handle(ctx, tx, e, f.clock.Now()) })
		})
	}
	close(start)
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatal(err)
		}
	}
	var headers, entries int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM user_txns`).Scan(&headers); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM user_txn_entries`).Scan(&entries); err != nil {
		t.Fatal(err)
	}
	if headers != 1 || entries != 2 {
		t.Fatalf("headers=%d entries=%d, want 1 and 2", headers, entries)
	}
}

func TestUserLedgerRejectsUnrepresentableAndInvalidDeposits(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	h := adapters.UserLedger{Ledger: f.ledger, IDs: f.ids, USDC: usdc, Hints: &hints{}}
	tooLarge := events.DepositCredited{AmountMicros: money.MicrosFromUint64(math.MaxUint64)}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		return h.Handle(ctx, tx, tooLarge, f.clock.Now())
	}); err == nil {
		t.Fatal("unrepresentable deposit error = nil")
	}
	payload, err := json.Marshal(tooLarge)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := adapters.DepositCreditedBalances(usdc)(payload); err == nil {
		t.Fatal("unrepresentable balance error = nil")
	}
	invalid := events.DepositCredited{AmountMicros: money.Micros{}}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		return h.Handle(ctx, tx, invalid, f.clock.Now())
	}); err == nil {
		t.Fatal("invalid deposit error = nil")
	}
}

type ledgerRowAtHint struct {
	f    fixture
	id   uuid.UUID
	rows []int
	keys []string
}

func (h *ledgerRowAtHint) PublishHint(ctx context.Context, key string, _ []byte) {
	var n int
	if err := h.f.pool.QueryRow(ctx, `SELECT count(*) FROM user_txns WHERE id = $1`, h.id).Scan(&n); err != nil {
		h.f.t.Error(err)
	}
	h.rows = append(h.rows, n)
	h.keys = append(h.keys, key)
}

func TestUserLedgerHintsTheUserOnceTheLedgerRowIsReadable(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user := f.user(t)
	e := events.DepositCredited{
		V: 1, DepositID: f.ids.NewV7(), UserID: user.UUID(),
		TxSignature: chain.Signature("deposit"), AmountMicros: money.MicrosFromUint64(25),
	}
	sent := &ledgerRowAtHint{f: f, id: e.DepositID}
	h := adapters.UserLedger{Ledger: f.ledger, IDs: f.ids, USDC: usdc, Hints: sent}
	rollback := errs.New(errs.CodeInternal, "test.rollback")
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		if err := h.Handle(ctx, tx, e, f.clock.Now()); err != nil {
			return err
		}
		return rollback
	}); !errors.Is(err, rollback) {
		t.Fatalf("rolled back handle error = %v, want %v", err, rollback)
	}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		posting, cancel := context.WithCancel(ctx)
		cancel()
		return h.Handle(posting, tx, e, f.clock.Now())
	}); err == nil {
		t.Fatal("handle with a failed ledger post error = nil")
	}
	if len(sent.keys) != 0 || f.count(t, "user_txns") != 0 {
		t.Fatalf("after rollbacks hints = %q, user_txns = %d; want none", sent.keys, f.count(t, "user_txns"))
	}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		return h.Handle(ctx, tx, e, f.clock.Now())
	}); err != nil {
		t.Fatal(err)
	}
	want := events.UserBalanceChangedHint(user)
	if !slices.Equal(sent.keys, []string{want}) || !slices.Equal(sent.rows, []int{1}) {
		t.Fatalf(
			"hints = %q with ledger rows %v at publish, want %q once with the row readable",
			sent.keys,
			sent.rows,
			want,
		)
	}
}
