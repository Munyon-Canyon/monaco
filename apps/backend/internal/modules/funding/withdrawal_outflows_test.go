package funding_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
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
