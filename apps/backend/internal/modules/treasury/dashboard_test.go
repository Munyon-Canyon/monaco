package treasury_test

import (
	"context"
	"math"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/port"
	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func dashboardOn(pool *pgxpool.Pool) adapters.Dashboard {
	return treasury.New(module.Deps{Pool: pool, Config: testkit.Config()}).DashboardOn(pool)
}

func day(d int) time.Time { return time.Date(2026, 9, d, 0, 0, 0, 0, time.UTC) }

func insertWalletTxn(t *testing.T, pool *pgxpool.Pool, gen *testkit.IDs, kind string, amount int64) {
	t.Helper()
	id := gen.NewV7()
	if _, err := pool.Exec(t.Context(), `INSERT INTO user_txns (id, user_id, kind, status, created_at)
		VALUES ($1, $2, $3, 'settled', $4)`, id, gen.NewV7(), kind, day(1)); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(t.Context(), `INSERT INTO user_txn_entries (txn_id, seq, account, asset, amount)
		VALUES ($1, 0, 'wallet', $2, $3)`, id, string(testkit.USDCMint), amount); err != nil {
		t.Fatal(err)
	}
}

func insertCabalTxn(
	t *testing.T, pool *pgxpool.Pool, gen *testkit.IDs, kind, status string, entries map[string]int64,
) {
	t.Helper()
	id := gen.NewV7()
	if _, err := pool.Exec(t.Context(), `INSERT INTO cabal_txns (id, cabal_id, kind, status, created_at, seq)
		VALUES ($1, $2, $3, $4, $5, 1)`, id, gen.NewV7(), kind, status, day(2)); err != nil {
		t.Fatal(err)
	}
	seq := 0
	for _, account := range []string{"treasury", "venue", "members"} {
		amount, ok := entries[account]
		if !ok {
			continue
		}
		if _, err := pool.Exec(t.Context(), `INSERT INTO cabal_txn_entries (txn_id, seq, account, asset, amount)
			VALUES ($1, $2, $3, $4, $5)`, id, seq, account, string(testkit.USDCMint), amount); err != nil {
			t.Fatal(err)
		}
		seq++
	}
}

func TestDashboard_LedgerTotals_SumsSettledMovementPerWeek(t *testing.T) {
	t.Parallel()
	pool, gen := testkit.DB(t), testkit.NewIDs(1)
	insertWalletTxn(t, pool, gen, "deposit", 30_000_000)
	insertWalletTxn(t, pool, gen, "deposit", 20_000_000)
	insertWalletTxn(t, pool, gen, "fund", -45_000_000)
	insertWalletTxn(t, pool, gen, "withdrawal", -5_000_000)
	insertCabalTxn(t, pool, gen, "swap", "settled", map[string]int64{"treasury": -9_000_000, "venue": 9_000_000})
	insertCabalTxn(t, pool, gen, "swap", "settled", map[string]int64{"treasury": 4_000_000, "venue": -4_000_000})
	insertCabalTxn(t, pool, gen, "swap", "failed", map[string]int64{"treasury": -1_000_000, "venue": 1_000_000})
	insertCabalTxn(t, pool, gen, "cash_out", "settled", map[string]int64{"treasury": -7_000_000, "members": 7_000_000})
	got, err := dashboardOn(pool).LedgerTotals(t.Context(), day(1), day(8), bucket.Week)
	flow := func(count int64, micros uint64) port.Flow {
		return port.Flow{Count: count, USDC: money.MicrosFromUint64(micros)}
	}
	want := port.LedgerBucket{
		Start: time.Date(2026, 8, 31, 0, 0, 0, 0, time.UTC), Deposits: flow(2, 50_000_000),
		Funds: flow(1, 45_000_000), CashOuts: flow(1, 7_000_000), Withdrawals: flow(1, 5_000_000),
		SwapBuy: money.MicrosFromUint64(9_000_000), SwapSell: money.MicrosFromUint64(4_000_000),
	}
	if err != nil || len(got) != 1 || got[0] != want {
		t.Fatalf("LedgerTotals() = %+v, %v, want one bucket %+v", got, err, want)
	}
}

func TestDashboard_LedgerTotals_ReadsNothingFromAnEmptyLedger(t *testing.T) {
	t.Parallel()
	got, err := dashboardOn(testkit.DB(t)).LedgerTotals(
		t.Context(), day(1), day(8), bucket.Week)
	if err != nil || len(got) != 0 {
		t.Fatalf("LedgerTotals() = %+v, %v, want no buckets", got, err)
	}
}

func TestDashboard_LedgerTotals_RefusesASumPastUint64(t *testing.T) {
	t.Parallel()
	pool, gen := testkit.DB(t), testkit.NewIDs(1)
	for range 3 {
		insertWalletTxn(t, pool, gen, "deposit", math.MaxInt64)
	}
	_, err := dashboardOn(pool).LedgerTotals(
		t.Context(), day(1), day(2), bucket.Day)
	if errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("LedgerTotals() error = %v, want decode_failed", err)
	}
}

func TestDashboard_PlatformBalanceTotal_SumsSettledWalletEntries(t *testing.T) {
	t.Parallel()
	pool, gen := testkit.DB(t), testkit.NewIDs(1)
	insertWalletTxn(t, pool, gen, "deposit", 30_000_000)
	insertWalletTxn(t, pool, gen, "withdrawal", -5_000_000)
	got, err := dashboardOn(pool).PlatformBalanceTotal(t.Context())
	if err != nil || got.Uint64() != 25_000_000 {
		t.Fatalf("PlatformBalanceTotal() = %v, %v, want 25000000", got, err)
	}
}

func TestDashboard_PlatformBalanceTotal_RefusesANegativeTotal(t *testing.T) {
	t.Parallel()
	pool, gen := testkit.DB(t), testkit.NewIDs(1)
	insertWalletTxn(t, pool, gen, "withdrawal", -1)
	if _, err := dashboardOn(pool).PlatformBalanceTotal(t.Context()); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("PlatformBalanceTotal() error = %v, want decode_failed", err)
	}
}

func TestDashboard_PlatformBalanceTotal_RefusesATotalPastInt64(t *testing.T) {
	t.Parallel()
	pool, gen := testkit.DB(t), testkit.NewIDs(1)
	insertWalletTxn(t, pool, gen, "deposit", math.MaxInt64)
	insertWalletTxn(t, pool, gen, "deposit", math.MaxInt64)
	if _, err := dashboardOn(pool).PlatformBalanceTotal(t.Context()); errs.CodeOf(err) != errs.CodeDecodeFailed {
		t.Fatalf("PlatformBalanceTotal() error = %v, want decode_failed", err)
	}
}

func TestDashboard_Reads_FailOnACancelledContext(t *testing.T) {
	t.Parallel()
	d := dashboardOn(testkit.DB(t))
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := d.LedgerTotals(ctx, time.Time{}, time.Time{}, bucket.Day); err == nil {
		t.Error("LedgerTotals() error = nil on a cancelled context")
	}
	if _, err := d.PlatformBalanceTotal(ctx); err == nil {
		t.Error("PlatformBalanceTotal() error = nil on a cancelled context")
	}
}
