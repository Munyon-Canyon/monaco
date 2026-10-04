package funding_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type hints struct{ keys []string }

func (h *hints) PublishHint(
	_ context.Context,
	key string,
	_ []byte,
) {
	h.keys = append(h.keys, key)
}

func TestCreditDeposit_writesOneDepositEventCursorAndHint(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC()
	h := &hints{}
	handler := app.NewCreditDepositHandler(
		db.New(pool, testkit.NewIDs(5), testkit.NewClock(now)),
		h,
	)
	cmd := app.CreditDeposit{
		ID:            testkit.NewIDs(6).NewV7(),
		UserID:        user.ID,
		WalletAddress: user.Address,
		TxSignature: chain.Signature(
			"5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW",
		),
		Amount: money.MicrosFromUint64(
			25_000_000,
		), Slot: 123, BlockTime: now, CreditedAt: now,
		CursorSignature: chain.Signature(
			"5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW",
		),
	}
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
	credited, err := handler.Handle(ctx, cmd)
	if err != nil || !credited {
		t.Fatalf("Handle = %v, %v, want true nil", credited, err)
	}
	credited, err = handler.Handle(ctx, cmd)
	if err != nil || credited {
		t.Fatalf("second Handle = %v, %v, want false nil", credited, err)
	}
	var deposits, eventCount int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM deposits`).Scan(&deposits); err != nil {
		t.Fatal(err)
	}
	const depositEvents = `SELECT count(*) FROM events WHERE type = $1`
	if err := pool.QueryRow(t.Context(), depositEvents, events.TypeDepositCredited).Scan(&eventCount); err != nil {
		t.Fatal(err)
	}
	if deposits != 1 || eventCount != 1 || len(h.keys) != 1 {
		t.Fatalf("deposits=%d events=%d hints=%v, want 1 1 one", deposits, eventCount, h.keys)
	}
	assertDepositCursor(t, pool, user.Address, cmd.CursorSignature)
	assertDepositHint(t, h, user.ID)
	assertDepositPayload(t, pool, cmd)
}

func assertDepositCursor(t *testing.T, pool *pgxpool.Pool, address chain.SolanaAddress, want chain.Signature) {
	t.Helper()
	var got string
	if err := pool.QueryRow(
		t.Context(),
		`SELECT last_signature FROM deposit_cursors WHERE wallet_address = $1`,
		address,
	).Scan(&got); err != nil {
		t.Fatal(err)
	}
	if got != string(want) {
		t.Fatalf("cursor = %q, want %q", got, want)
	}
}

func assertDepositHint(t *testing.T, h *hints, userID ids.UserID) {
	t.Helper()
	want := "user." + userID.String() + ".balance_changed"
	if h.keys[0] != want {
		t.Fatalf("hint = %q, want %q", h.keys[0], want)
	}
}

func assertDepositPayload(t *testing.T, pool *pgxpool.Pool, want app.CreditDeposit) {
	t.Helper()
	var payload []byte
	if err := pool.QueryRow(
		t.Context(),
		`SELECT payload FROM events WHERE type = $1`,
		events.TypeDepositCredited,
	).Scan(&payload); err != nil {
		t.Fatal(err)
	}
	var got events.DepositCredited
	if err := json.Unmarshal(payload, &got); err != nil {
		t.Fatal(err)
	}
	if got.DepositID != want.ID || got.UserID != want.UserID.UUID() ||
		got.WalletAddress != want.WalletAddress || got.AmountMicros != want.Amount ||
		got.TxSignature != want.TxSignature || got.Slot != want.Slot {
		t.Fatalf("deposit.credited = %+v, want command payload", got)
	}
}

func TestCreditDeposit_duplicateDoesNotMoveCursorBackward(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC()
	handler := app.NewCreditDepositHandler(db.New(pool, testkit.NewIDs(13), testkit.NewClock(now)), &hints{})
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
	newer := app.CreditDeposit{
		ID:            testkit.NewIDs(14).NewV7(),
		UserID:        user.ID,
		WalletAddress: user.Address,
		TxSignature: chain.Signature(
			"5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW",
		),
		Amount:          money.MicrosFromUint64(1),
		Slot:            2,
		BlockTime:       now,
		CreditedAt:      now,
		CursorSignature: chain.Signature("newer"),
	}
	if _, err := handler.Handle(ctx, newer); err != nil {
		t.Fatal(err)
	}
	duplicate := newer
	duplicate.CursorSignature = chain.Signature("older")
	duplicate.Slot = 1
	if credited, err := handler.Handle(ctx, duplicate); err != nil || credited {
		t.Fatalf("duplicate = %v, %v", credited, err)
	}
	var cursor string
	if err := pool.QueryRow(
		t.Context(),
		`SELECT last_signature FROM deposit_cursors WHERE wallet_address = $1`,
		user.Address,
	).Scan(&cursor); err != nil {
		t.Fatal(err)
	}
	if cursor != "newer" {
		t.Fatalf("cursor = %q, want newer", cursor)
	}
}

func TestCreditDepositDefersCursor(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC()
	handler := app.NewCreditDepositHandler(db.New(pool, testkit.NewIDs(17), testkit.NewClock(now)), &hints{})
	cmd := app.CreditDeposit{
		ID: testkit.NewIDs(18).NewV7(), UserID: user.ID, WalletAddress: user.Address,
		TxSignature: "signature", Amount: money.MicrosFromUint64(1), Slot: 1, CreditedAt: now,
		CursorSignature: "signature", DeferCursor: true,
	}
	ctx := observability.WithActor(t.Context(), "system:funding.deposits")
	if credited, err := handler.Handle(ctx, cmd); err != nil || !credited {
		t.Fatalf("Handle = %v, %v, want true nil", credited, err)
	}
	var cursors int
	const cursorCount = `SELECT count(*) FROM deposit_cursors WHERE wallet_address = $1`
	if err := pool.QueryRow(t.Context(), cursorCount, user.Address).Scan(&cursors); err != nil {
		t.Fatal(err)
	}
	if cursors != 0 {
		t.Fatalf("cursors = %d, want 0", cursors)
	}
}

func TestCreditDeposit_rollsBackOnEachWriteFailure(t *testing.T) {
	t.Parallel()
	check := func(t *testing.T, table string) {
		t.Helper()
		pool := testkit.DB(t)
		user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
		now := clock.Real{}.Now().UTC()
		if _, err := pool.Exec(t.Context(), "ALTER TABLE "+table+" RENAME TO "+table+"_gone"); err != nil {
			t.Fatal(err)
		}
		cmd := app.CreditDeposit{
			ID: testkit.NewIDs(7).NewV7(), UserID: user.ID, WalletAddress: user.Address,
			TxSignature: chain.Signature(
				"5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW",
			),
			Amount: money.MicrosFromUint64(1), BlockTime: now, CreditedAt: now,
		}
		_, err := app.NewCreditDepositHandler(db.New(pool, testkit.NewIDs(8), testkit.NewClock(now)), &hints{}).
			Handle(observability.WithActor(t.Context(), "system:poller.funding.deposits"), cmd)
		if err == nil {
			t.Fatal("Handle error = nil")
		}
	}
	t.Run("deposits", func(t *testing.T) {
		t.Parallel()
		check(t, "deposits")
	})
	t.Run("cursor", func(t *testing.T) {
		t.Parallel()
		check(t, "deposit_cursors")
	})
}

func TestCreditDeposit_rollsBackWhenTheEventHasNoActor(t *testing.T) {
	t.Parallel()
	testCreditDepositRollsBackWhenTheEventHasNoActor(t)
}

func testCreditDepositRollsBackWhenTheEventHasNoActor(t *testing.T) {
	t.Helper()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC()
	cmd := app.CreditDeposit{
		ID: testkit.NewIDs(9).NewV7(), UserID: user.ID, WalletAddress: user.Address,
		TxSignature: chain.Signature(
			"5VERv8NMvzbJMEkV8xnrLkEaWRtSz9CosKDYjCJjBRnbJLgp8uirBgmQpjKhoR4tjF3ZpRzrFmBV6UjKdiSZkQUW",
		),
		Amount: money.MicrosFromUint64(1), BlockTime: now, CreditedAt: now,
	}
	_, err := app.NewCreditDepositHandler(db.New(pool, testkit.NewIDs(10), testkit.NewClock(now)), &hints{}).
		Handle(t.Context(), cmd)
	if err == nil {
		t.Fatal("Handle error = nil")
	}
}

func TestCreditDeposit_refusesZeroAmount(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	_, err := app.NewCreditDepositHandler(db.New(pool, testkit.NewIDs(11), clock.Real{}), &hints{}).Handle(
		observability.WithActor(t.Context(), "system:poller.funding.deposits"),
		app.CreditDeposit{ID: testkit.NewIDs(12).NewV7(), UserID: user.ID, WalletAddress: user.Address},
	)
	if err == nil {
		t.Fatal("Handle error = nil")
	}
}

func TestCreditDepositStoresNilBlockTime(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC()
	cmd := app.CreditDeposit{
		ID: testkit.NewIDs(15).NewV7(), UserID: user.ID, WalletAddress: user.Address,
		TxSignature: chain.Signature("nil-block-time"), Amount: money.MicrosFromUint64(1), CreditedAt: now,
	}
	if _, err := app.NewCreditDepositHandler(db.New(pool, testkit.NewIDs(16), testkit.NewClock(now)), &hints{}).
		Handle(observability.WithActor(t.Context(), "system:poller.funding.deposits"), cmd); err != nil {
		t.Fatal(err)
	}
	var blockTime *time.Time
	err := pool.QueryRow(t.Context(), `SELECT block_time FROM deposits WHERE id = $1`, cmd.ID).Scan(&blockTime)
	if err != nil {
		t.Fatal(err)
	}
	if blockTime != nil {
		t.Fatalf("block_time = %s, want NULL", blockTime)
	}
}
