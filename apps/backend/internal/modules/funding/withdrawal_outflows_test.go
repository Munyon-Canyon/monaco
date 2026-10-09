package funding_test

import (
	"context"
	"maps"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const seedWithdrawal = `INSERT INTO withdrawals (id, user_id, amount_micros, to_address, status, signed_tx,
	tx_signature, last_valid_block_height, fail_code, created_at, submitted_at, completed_at)
VALUES ($1::uuid, $2, $3, 'to', $4::text,
	CASE WHEN $4 IN ('submitted', 'confirmed') THEN '\x01'::bytea END,
	CASE WHEN $4 IN ('submitted', 'confirmed') THEN $1::uuid::text END,
	CASE WHEN $4 IN ('submitted', 'confirmed') THEN 9 END,
	CASE WHEN $4 = 'failed' THEN 'privy_unavailable' END,
	$5, CASE WHEN $4 IN ('submitted', 'confirmed') THEN $5::timestamptz END,
	CASE WHEN $4 IN ('confirmed', 'failed') THEN $5::timestamptz END)`

func TestWithdrawalOutflows_sumsCreatedAndSubmitted(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	other := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now()
	for _, row := range []struct {
		user   ids.UserID
		micros int64
		status string
	}{
		{user.ID, 1_000_000, "created"},
		{user.ID, 2_000_000, "submitted"},
		{user.ID, 4_000_000, "confirmed"},
		{user.ID, 8_000_000, "failed"},
		{other.ID, 16_000_000, "created"},
	} {
		if _, err := pool.Exec(t.Context(), seedWithdrawal,
			ids.Real{}.NewV7(), row.user.UUID(), row.micros, row.status, now); err != nil {
			t.Fatal(err)
		}
	}
	got, err := (app.WithdrawalOutflows{Reads: pool}).InFlightMicros(t.Context(), user.ID)
	if err != nil || got.String() != "3000000" {
		t.Fatalf("InFlightMicros = %v, %v, want 3000000", got, err)
	}
	if _, err := pool.Exec(t.Context(), `ALTER TABLE withdrawals RENAME TO withdrawals_gone`); err != nil {
		t.Fatal(err)
	}
	if _, err := (app.WithdrawalOutflows{Reads: pool}).InFlightMicros(t.Context(), user.ID); err == nil {
		t.Fatal("InFlightMicros error = nil")
	}
}

func TestWithdrawalOutflows_lastChangeIsTheUsersLatestMove(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	other := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	outflows, q := app.WithdrawalOutflows{Reads: pool}, sqlc.New(pool)
	at := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	tick := func() time.Time { at = at.Add(time.Second); return at }
	assertLast := func(want time.Time) {
		t.Helper()
		if got, err := outflows.LastChange(t.Context(), user.ID); err != nil || !got.Equal(want) {
			t.Fatalf("LastChange = %v, %v, want %v", got, err, want)
		}
	}
	insert := func(owner ids.UserID) uuid.UUID {
		t.Helper()
		id := ids.Real{}.NewV7()
		if err := q.InsertWithdrawal(t.Context(), sqlc.InsertWithdrawalParams{
			ID: id, UserID: owner.UUID(), ToAddress: "to", AmountMicros: "1000000", CreatedAt: tick(),
		}); err != nil {
			t.Fatal(err)
		}
		return id
	}
	moved := func(n int64, err error) {
		t.Helper()
		if err != nil || n != 1 {
			t.Fatalf("move = %d, %v, want one row", n, err)
		}
		assertLast(at)
	}
	assertLast(time.Unix(0, 0))
	confirmed := insert(user.ID)
	assertLast(at)
	moved(q.SubmitWithdrawal(t.Context(), sqlc.SubmitWithdrawalParams{
		ID: confirmed, SignedTx: []byte{1}, TxSignature: confirmed.String(), LastValidBlockHeight: "9",
		SubmittedAt: tick(),
	}))
	moved(q.ConfirmWithdrawal(t.Context(), sqlc.ConfirmWithdrawalParams{ID: confirmed, CompletedAt: tick()}))
	failed := insert(user.ID)
	assertLast(at)
	moved(q.FailWithdrawal(t.Context(), sqlc.FailWithdrawalParams{
		ID: failed, FailCode: "privy_unavailable", CompletedAt: tick(),
	}))
	latest := at
	insert(other.ID)
	assertLast(latest)
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := outflows.LastChange(ctx, user.ID); err == nil {
		t.Fatal("LastChange on a canceled context error = nil")
	}
}

func TestWithdrawalOutflows_submittedIsTheUsersSubmittedAmountBySignature(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	other := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now()
	want := map[chain.Signature]money.Micros{}
	seed := func(owner ids.UserID, micros int64, status string) uuid.UUID {
		t.Helper()
		id := ids.Real{}.NewV7()
		if _, err := pool.Exec(t.Context(), seedWithdrawal, id, owner.UUID(), micros, status, now); err != nil {
			t.Fatal(err)
		}
		return id
	}
	for i, status := range []string{"created", "submitted", "submitted", "confirmed", "failed"} {
		micros := int64(i+1) * 1_000_000
		if id := seed(user.ID, micros, status); status == "submitted" {
			want[chain.Signature(id.String())] = money.MicrosFromUint64(uint64(micros))
		}
	}
	seed(other.ID, 16_000_000, "submitted")
	outflows := app.WithdrawalOutflows{Reads: pool}
	if got, err := outflows.Submitted(t.Context(), user.ID); err != nil || !maps.Equal(got, want) {
		t.Fatalf("Submitted = %v, %v, want %v", got, err, want)
	}
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := outflows.Submitted(ctx, user.ID); err == nil {
		t.Fatal("Submitted on a canceled context error = nil")
	}
	huge := seed(user.ID, 1_000_000, "submitted")
	if _, err := pool.Exec(t.Context(), `UPDATE withdrawals SET amount_micros = 99999999999999999999 WHERE id = $1`,
		huge); err != nil {
		t.Fatal(err)
	}
	if _, err := outflows.Submitted(t.Context(), user.ID); err == nil {
		t.Fatal("Submitted with an amount past uint64 error = nil")
	}
}

func TestWithdrawalReads_returnsTheReadError(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{})
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := (app.WithdrawalReads{Reads: pool}).OpenWithdrawals(
		ctx,
		user.ID,
		port.WithdrawalPage{Limit: 1},
	); err == nil {
		t.Fatal("OpenWithdrawals on a canceled context error = nil")
	}
}
