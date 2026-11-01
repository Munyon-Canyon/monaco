package treasury_test

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func (f fixture) swapRow(t *testing.T, cabal ids.CabalID, swap uuid.UUID, status string, at time.Time) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO swaps (id, source_kind, source_id, cabal_id, treasury_address,
		action, symbol, in_mint, out_mint, out_decimals, in_amount, slippage_bps, source_batch_size, status,
		created_at, confirmed_at, updated_at)
		VALUES ($1, 'proposal', $2, $3, 'treasury', 'buy', 'AAPLx', 'usdc', 'aapl', 6, 1, 100, 1, $5,
		$4, $4, $4)`, swap, f.ids.NewV7(), cabal.UUID(), at, status); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) bridgeStepRow(t *testing.T, swap uuid.UUID, status, action string, at time.Time) {
	t.Helper()
	var privy, failure any
	if action != "" {
		privy = action
	}
	if status == "failed" {
		failure = "bridge refused"
	}
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO trade_steps (id, trade_id, step, network, privy_action_id,
		amount_micros, status, error, created_at, updated_at) VALUES ($1, $2, 'bridge_out', 'base', $3, 10000000, $4, $5, $6, $6)`,
		f.ids.NewV7(), swap, privy, status, failure, at); err != nil {
		t.Fatal(err)
	}
}

func (f fixture) reconcile(t *testing.T) int {
	t.Helper()
	report, err := adapters.LedgerReconcile{Reads: f.pool, Clock: f.clock, Every: time.Minute}.Tick(f.ctx())
	if err != nil {
		t.Fatal(err)
	}
	return report.Changed
}

func TestLedgerReconcile_alertsOnAConfirmedSwapUnbookedForMoreThanTenMinutes(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	fresh, stale := f.ids.NewV7(), f.ids.NewV7()
	f.swapRow(t, cabal, fresh, "confirmed", f.clock.Now().Add(-9*time.Minute))
	f.swapRow(t, cabal, stale, "confirmed", f.clock.Now().Add(-11*time.Minute))

	if got := f.reconcile(t); got != 1 {
		t.Fatalf("unbooked = %d, want only the swap confirmed 11 minutes ago", got)
	}
	lines := f.logs.Lines(observability.TreasuryLedgerUnbooked.Name, "level", "kind", "id", "cabal_id")
	if len(lines) != 1 {
		t.Fatalf("alerts = %q, want one", lines)
	}

	if err := f.confirm(t, buy(cabal, stale, 60_000_000, 3_000_000, 0)); err != nil {
		t.Fatal(err)
	}
	f.clock.Advance(2 * time.Minute)
	if got := f.reconcile(t); got != 1 {
		t.Fatalf("unbooked = %d once the stale swap is booked and the fresh one has aged, want the fresh one", got)
	}
	if err := f.confirm(t, buy(cabal, fresh, 10_000_000, 500_000, 0)); err != nil {
		t.Fatal(err)
	}
	if got := f.reconcile(t); got != 0 {
		t.Fatalf("unbooked = %d after both swaps are booked, want none", got)
	}
}

func TestLedgerReconcile_alertsOnABridgeStepWhoseLandingOrReturnIsUnbooked(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	aged := f.clock.Now().Add(-11 * time.Minute)
	landed := newBridgeRun(t, f, cabal, "bridge_out")
	returned := newBridgeRun(t, f, cabal, "bridge_out")
	noAction := newBridgeRun(t, f, cabal, "bridge_out")
	for _, run := range []bridgeRun{landed, returned, noAction} {
		f.swapRow(t, cabal, run.swap, "created", aged)
	}
	f.bridgeStepRow(t, landed.swap, "succeeded", "privy-landed", aged)
	f.bridgeStepRow(t, returned.swap, "failed", "privy-returned", aged)
	f.bridgeStepRow(t, noAction.swap, "failed", "", aged)

	if got := f.reconcile(t); got != 2 {
		t.Fatalf("unbooked = %d, want the succeeded landing and the failed return, not the failure with no action", got)
	}
	lines := f.logs.Lines(observability.TreasuryLedgerUnbooked.Name, "level", "kind")
	want := []string{"ERROR bridge_out succeeded", "ERROR bridge_out failed"}
	slices.Sort(lines)
	slices.Sort(want)
	if !slices.Equal(lines, want) {
		t.Fatalf("alerts = %q, want error alerts %q for the landing and the return", lines, want)
	}

	landed.handle(t, landed.event("succeeded", "privy-landed", 10_000_000, 7_500))
	if got := f.reconcile(t); got != 1 {
		t.Fatalf("unbooked = %d after the landing is booked, want the return", got)
	}
	returned.handle(t, returned.event("failed", "privy-returned", 10_000_000, 0))
	if got := f.reconcile(t); got != 0 {
		t.Fatalf("unbooked = %d after both steps are booked, want none", got)
	}
}

type failingReads struct {
	sqlc.DBTX
	okQueries int
}

func (r *failingReads) Query(ctx context.Context, sql string, args ...any) (pgx.Rows, error) {
	if r.okQueries == 0 {
		return nil, errs.New(errs.CodeDBUnavailable, "test.reads")
	}
	r.okQueries--
	return r.DBTX.Query(ctx, sql, args...)
}

func TestLedgerReconcile_failsTheTickWhenAReadFails(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	aged := f.clock.Now().Add(-11 * time.Minute)
	run := newBridgeRun(t, f, cabal, "bridge_out")
	f.swapRow(t, cabal, run.swap, "created", aged)
	f.bridgeStepRow(t, run.swap, "succeeded", "privy-out", aged)
	for ok := range 3 {
		reads := &failingReads{DBTX: f.pool, okQueries: ok}
		if _, err := (adapters.LedgerReconcile{Reads: reads, Clock: f.clock}).Tick(f.ctx()); err == nil {
			t.Errorf("tick after %d good reads = nil, want the read error", ok)
		}
	}
}

func TestLedgerReconcile_failsTheTickOnALandingBookedOnAnUnreadableSwap(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	aged := f.clock.Now().Add(-11 * time.Minute)
	run := newBridgeRun(t, f, cabal, "bridge_out")
	f.swapRow(t, cabal, run.swap, "created", aged)
	f.bridgeStepRow(t, run.swap, "succeeded", "reused:not-a-swap", aged)
	if _, err := (adapters.LedgerReconcile{Reads: f.pool, Clock: f.clock}).Tick(f.ctx()); err == nil {
		t.Fatal("tick = nil, want an error for a reused landing that names no swap")
	}
}

func TestLedgerReconcile_alertsOnlyOnTheFirstTwentyUnbookedAndCountsTheRest(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	for range 21 {
		f.swapRow(t, cabal, f.ids.NewV7(), "confirmed", f.clock.Now().Add(-11*time.Minute))
	}
	if got := f.reconcile(t); got != 21 {
		t.Fatalf("unbooked = %d, want 21", got)
	}
	if lines := f.logs.Lines(observability.TreasuryLedgerUnbooked.Name, "unbooked_total"); len(lines) != 20 {
		t.Fatalf("alerts = %d, want 20", len(lines))
	}
}
