package main

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	tradingapp "github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const cashWallet = "0x3318a72025dafedf50c63fe0d087c0e2653310e5"

type cashWallets struct{}

func (cashWallets) EnsureTreasuryEVMWallet(context.Context, ids.CabalID) (multichain.EVMWallet, error) {
	address, err := chain.ParseEVMAddress(cashWallet)
	return multichain.EVMWallet{ID: "evm-wallet", Address: address}, err
}

type cashBalance struct{ micros []uint64 }

func (c *cashBalance) Balance(context.Context, chain.Network, chain.EVMAddress) (money.Micros, error) {
	got := c.micros[0]
	if len(c.micros) > 1 {
		c.micros = c.micros[1:]
	}
	return money.MicrosFromUint64(got), nil
}

func fakeCashPorts(cash tradingapp.EVMCash) cashPorts {
	return func(config.Config, *pgxpool.Pool, *db.UnitOfWork, clock.Clock) cashAdapters {
		return cashAdapters{Wallets: cashWallets{}, Cash: cash}
	}
}

func returnCashTool(t *testing.T, input string, balances ...uint64) (tool, *pgxpool.Pool, *testkit.IDs, time.Time) {
	t.Helper()
	swaps, pool, identifiers, now := parkedSwapsTool(t, input)
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=" + testkit.NATSURL(), "USER=alice",
	}
	_ = swaps
	return swapsToolWith(
			environ,
			testkit.NewClock(now),
			strings.NewReader(input),
			fakeCashPorts(&cashBalance{micros: balances}),
		),
		pool, identifiers, now
}

func seededStrandedBuy(t *testing.T, pool *pgxpool.Pool, identifiers *testkit.IDs, at time.Time) uuid.UUID {
	t.Helper()
	swap := seededRoutedSwap(t, pool, identifiers, at, false)
	if _, err := pool.Exec(t.Context(), `UPDATE swaps SET status = 'failed', failed_at = $2,
		failure_code = 'never_submitted' WHERE id = $1`, swap, at); err != nil {
		t.Fatal(err)
	}
	if _, err := pool.Exec(
		t.Context(),
		`UPDATE trade_steps SET arrived_micros = 24992500, arrival_tx = 'arrival-' || trade_id::text
		WHERE trade_id = $1 AND step = 'bridge_out'`,
		swap,
	); err != nil {
		t.Fatal(err)
	}
	return swap
}

func TestSwapsReturnEVMCash_queuesTheReturnPrintsThePlanAndRecordsTheOperator(t *testing.T) {
	t.Parallel()
	swaps, pool, identifiers, now := returnCashTool(t, "y\n", 24_000_000)
	swap := seededStrandedBuy(t, pool, identifiers, now)

	code, out, stderr := runSwaps(
		t,
		swaps,
		"return-evm-cash",
		"--swap",
		swap.String(),
		"--reason",
		"stranded since 2026-10-12",
	)

	if code != 0 {
		t.Fatalf("return-evm-cash = %d, stderr %q", code, stderr)
	}
	wantContains(t, "stderr", stderr, "[y/N]")
	wantContains(t, "stdout", out,
		"plan swap="+swap.String()+" requested="+swap.String(),
		"network=base landed_micros=24992500 on_chain_micros=24000000 amount_micros=24000000",
		"queued swap="+swap.String(), "status=pending",
	)
	var step, action string
	if err := pool.QueryRow(t.Context(), `SELECT status || ' ' || amount_micros FROM trade_steps
		WHERE trade_id = $1 AND step = 'bridge_back'`, swap).Scan(&step); err != nil || step != "pending 24000000" {
		t.Fatalf("bridge_back = %q, %v, want pending 24000000", step, err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT actor || ' ' || reason FROM swap_operator_actions
		WHERE swap_id = $1 AND action = 'return_evm_cash'`, swap).Scan(&action); err != nil ||
		action != "operator:alice stranded since 2026-10-12" {
		t.Fatalf("operator action = %q, %v", action, err)
	}
	code, _, stderr = runSwaps(t, swaps, "return-evm-cash", "--swap", swap.String(), "--reason", "again", "--yes")
	if code != 1 {
		t.Fatalf("second run = %d, want 1", code)
	}
	wantContains(t, "second run stderr", stderr, "stranded_cash_taken")
}

func TestSwapsReturnEVMCash_actsOnTheHolderOfReusedCash(t *testing.T) {
	t.Parallel()
	swaps, pool, identifiers, now := returnCashTool(t, "", 24_992_500)
	first := seededStrandedBuy(t, pool, identifiers, now)
	second := seededStrandedBuy(t, pool, identifiers, now)
	if _, err := pool.Exec(t.Context(), `UPDATE trade_steps SET privy_action_id = $2
		WHERE trade_id = $1 AND step = 'bridge_out'`, second, "reused:"+first.String()); err != nil {
		t.Fatal(err)
	}

	code, out, stderr := runSwaps(t, swaps, "return-evm-cash", "--swap", first.String(), "--reason", "reused", "--yes")

	if code != 0 {
		t.Fatalf("return-evm-cash = %d, stderr %q", code, stderr)
	}
	wantContains(t, "stdout", out, "plan swap="+second.String()+" requested="+first.String())
	var onFirst int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM trade_steps WHERE trade_id = $1 AND step = 'bridge_back'`,
		first).
		Scan(&onFirst); err != nil ||
		onFirst != 0 {
		t.Fatalf("bridge_back steps on the swap that gave its cash away = %d, %v, want 0", onFirst, err)
	}
}

func TestSwapsReturnEVMCash_refusesBadArgumentsAnUnknownSwapADeclinedPromptAndMissingPorts(t *testing.T) {
	t.Parallel()
	swaps, pool, identifiers, now := returnCashTool(t, "n\n", 24_000_000)
	swap := seededStrandedBuy(t, pool, identifiers, now)
	for _, args := range [][]string{
		{"return-evm-cash", "--swap", swap.String()},
		{"return-evm-cash", "--swap", "not-a-uuid", "--reason", "x"},
		{"return-evm-cash", "--swap", swap.String(), "--reason", "x", "extra"},
	} {
		if code, _, stderr := runSwaps(t, swaps, args...); code != 2 || !strings.Contains(stderr, "return-evm-cash") {
			t.Errorf("%v = %d, stderr %q, want the usage and exit 2", args, code, stderr)
		}
	}
	code, _, stderr := runSwaps(t, swaps, "return-evm-cash", "--swap", uuid.NewString(), "--reason", "x", "--yes")
	if code != 1 || !strings.Contains(stderr, "swap_not_found") {
		t.Fatalf("unknown swap = %d, stderr %q, want swap_not_found", code, stderr)
	}
	code, _, stderr = runSwaps(t, swaps, "return-evm-cash", "--swap", swap.String(), "--reason", "x")
	if code != 1 || !strings.Contains(stderr, "cancelled") {
		t.Fatalf("declined prompt = %d, stderr %q, want cancelled", code, stderr)
	}
	var steps int
	if err := pool.QueryRow(t.Context(), `SELECT count(*) FROM trade_steps WHERE step = 'bridge_back'`).
		Scan(&steps); err != nil ||
		steps != 0 {
		t.Fatalf("bridge_back steps after the declined prompt = %d, %v, want 0", steps, err)
	}
}

func TestSwapsReturnEVMCash_theLivePortsComeFromTheRegisteredModules(t *testing.T) {
	t.Parallel()
	swaps, _, _, _ := parkedSwapsTool(t, "")

	code, _, stderr := runSwaps(t, swaps, "return-evm-cash", "--swap", uuid.NewString(), "--reason", "x", "--yes")

	if code != 1 || !strings.Contains(stderr, "swap_not_found") {
		t.Fatalf("return-evm-cash on live ports = %d, stderr %q, want swap_not_found", code, stderr)
	}
}

func TestSwapsReturnEVMCash_reportsAMissingDatabase(t *testing.T) {
	t.Parallel()
	_, pool, identifiers, now := parkedSwapsTool(t, "")
	missing := strings.Replace(pool.Config().ConnString(), "/"+pool.Config().ConnConfig.Database,
		"/missing_"+identifiers.NewV7().String()[:8], 1)
	gone := swapsToolWith([]string{"MONACO_ENV=test", "DATABASE_URL=" + missing, "NATS_URL=" + testkit.NATSURL()},
		testkit.NewClock(now), strings.NewReader(""), fakeCashPorts(&cashBalance{micros: []uint64{1}}))

	args := []string{"return-evm-cash", "--swap", uuid.NewString(), "--reason", "x", "--yes"}
	if code, _, stderr := runSwaps(t, gone, args...); code != 1 || stderr == "" {
		t.Fatalf("return-evm-cash on a missing database = %d, stderr %q, want 1", code, stderr)
	}
}

func TestSwapsReturnEVMCash_reportsAReturnThatFailsAfterTheConfirmation(t *testing.T) {
	t.Parallel()
	swaps, pool, identifiers, now := returnCashTool(t, "", 24_000_000, 0)
	swap := seededStrandedBuy(t, pool, identifiers, now)

	code, _, stderr := runSwaps(t, swaps, "return-evm-cash", "--swap", swap.String(), "--reason", "x", "--yes")

	if code != 1 || !strings.Contains(stderr, "insufficient_funds") {
		t.Fatalf("return-evm-cash = %d, stderr %q, want insufficient_funds once the wallet is empty", code, stderr)
	}
}
