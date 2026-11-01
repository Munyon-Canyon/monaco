package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	tradingapp "github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type receiptStub struct{ fill tradingapp.EVMFill }

func (r receiptStub) Receipt(context.Context, ids.SwapID) (tradingapp.EVMFill, error) {
	return r.fill, nil
}

func receiptPorts(fill tradingapp.EVMFill) cashPorts {
	return func(config.Config, *pgxpool.Pool, *db.UnitOfWork, clock.Clock) cashAdapters {
		return cashAdapters{Receipts: receiptStub{fill: fill}}
	}
}

func receiptMissingTool(t *testing.T, fill tradingapp.EVMFill) (tool, uuid.UUID, string, *pgxpool.Pool) {
	t.Helper()
	pool := testkit.DB(t)
	now := time.Now().UTC().Truncate(time.Second).Add(-time.Hour)
	identifiers := testkit.NewIDs(1)
	swap := seededRoutedSwap(t, pool, identifiers, now, false)
	hash := "0x" + strings.Repeat("ab", 32)
	q := sqlc.New(pool)
	if n, err := q.MarkSubmittedEVM(t.Context(), sqlc.MarkSubmittedEVMParams{
		ID: swap, TxHash: hash, SubmittedAt: now,
	}); err != nil || n != 1 {
		t.Fatalf("MarkSubmittedEVM = %d, %v", n, err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO trade_steps (id, trade_id, step, network, privy_action_id,
		amount_micros, status, created_at, updated_at) VALUES ($1, $2, 'swap', 'base', $3, 25000000,
		'submitted', $4, $4)`, identifiers.NewV7(), swap, hash, now); err != nil {
		t.Fatal(err)
	}
	if _, err := q.ParkRouted(t.Context(), sqlc.ParkRoutedParams{
		ID: swap, At: now, Code: tradingapp.ParkedSwapReceiptMissing,
	}); err != nil {
		t.Fatal(err)
	}
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=" + testkit.NATSURL(), "USER=alice",
	}
	return swapsToolWith(environ, testkit.NewClock(now), strings.NewReader(""), receiptPorts(fill)), swap, hash, pool
}

func swapStatus(t *testing.T, pool *pgxpool.Pool, swap uuid.UUID) string {
	t.Helper()
	var status string
	if err := pool.QueryRow(t.Context(), `SELECT status FROM swaps WHERE id = $1`, swap).Scan(&status); err != nil {
		t.Fatal(err)
	}
	return status
}

func TestSwapsForceResolve_refusesARoutedSwapTheReceiptContradictsUntilOverridden(t *testing.T) {
	t.Parallel()
	swaps, swap, hash, pool := receiptMissingTool(t, tradingapp.EVMFill{State: tradingapp.FillSucceeded, Out: 7})
	args := []string{"force-resolve", "--signature", hash, "--to", "failed", "--reason", "gone", "--yes"}

	code, _, stderr := runSwaps(t, swaps, args...)

	if code != 1 {
		t.Fatalf("force-resolve = %d, stderr %q, want 1", code, stderr)
	}
	wantContains(t, "force-resolve stderr", stderr, "its receipt shows it landed successfully", "--override")
	if status := swapStatus(t, pool, swap); status != "submitted" {
		t.Fatalf("swap is %s, want it left submitted", status)
	}
	code, out, stderr := runSwaps(t, swaps, append(args, "--override")...)
	if code != 0 || !strings.Contains(out, "status=failed") {
		t.Fatalf("overridden force-resolve = %d, stdout %q, stderr %q", code, out, stderr)
	}
}

func TestSwapsFailParked_refusesAReceiptMissingSwapThatLanded(t *testing.T) {
	t.Parallel()
	swaps, swap, _, pool := receiptMissingTool(t, tradingapp.EVMFill{State: tradingapp.FillSucceeded, Out: 7})

	code, _, stderr := runSwaps(t, swaps, "fail-parked", "--swap", swap.String(), "--reason", "gone", "--yes")

	if code != 1 {
		t.Fatalf("fail-parked = %d, stderr %q, want 1", code, stderr)
	}
	wantContains(t, "fail-parked stderr", stderr, "landed successfully")
	if status := swapStatus(t, pool, swap); status != "submitted" {
		t.Fatalf("swap is %s, want it left submitted", status)
	}
}

func TestSwapsFailParked_failsAReceiptMissingSwapThatReverted(t *testing.T) {
	t.Parallel()
	swaps, swap, _, pool := receiptMissingTool(t, tradingapp.EVMFill{State: tradingapp.FillReverted})

	code, out, stderr := runSwaps(t, swaps, "fail-parked", "--swap", swap.String(), "--reason", "reverted", "--yes")

	if code != 0 || !strings.Contains(out, "status=failed") {
		t.Fatalf("fail-parked = %d, stdout %q, stderr %q", code, out, stderr)
	}
	if status := swapStatus(t, pool, swap); status != "failed" {
		t.Fatalf("swap is %s, want failed", status)
	}
}
