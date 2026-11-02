package main

import (
	"context"
	"strings"
	"testing"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	tradingapp "github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const arrivalHash = "0x00000000000000000000000000000000000000000000000000000000000000ab"

type arrivalTransfers struct {
	amount uint64
	found  []bool
	reads  int
}

func (a *arrivalTransfers) Transfer(
	context.Context, chain.Network, chain.EVMAddress, chain.EVMAddress, string,
) (tradingapp.EVMArrival, bool, error) {
	found := a.found[min(a.reads, len(a.found)-1)]
	a.reads++
	return tradingapp.EVMArrival{Amount: money.MicrosFromUint64(a.amount), TxHash: arrivalHash}, found, nil
}

func chooseTool(t *testing.T, input string, transfers tradingapp.EVMTransfers) (tool, *pgxpool.Pool, uuid.UUID) {
	t.Helper()
	_, pool, identifiers, now := parkedSwapsTool(t, input)
	swap := seededRoutedSwap(t, pool, identifiers, now, false)
	if _, err := pool.Exec(t.Context(), `INSERT INTO trade_routes (trade_id, comparison, winner_listing_id, created_at)
		SELECT $1, '{}'::jsonb, id, $2 FROM asset_listings LIMIT 1`, swap, now); err != nil {
		t.Fatal(err)
	}
	ports := func(config.Config, *pgxpool.Pool, *db.UnitOfWork, clock.Clock) cashAdapters {
		return cashAdapters{Wallets: cashWallets{}, Transfers: transfers}
	}
	environ := []string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=" + testkit.NATSURL(), "USER=alice",
	}
	return swapsToolWith(environ, testkit.NewClock(now), strings.NewReader(input), ports), pool, swap
}

func chosenRow(t *testing.T, pool *pgxpool.Pool, swap uuid.UUID) (string, string) {
	t.Helper()
	var tx, action string
	if err := pool.QueryRow(t.Context(), `SELECT coalesce(arrival_tx, '') FROM trade_steps
		WHERE trade_id = $1 AND step = 'bridge_out'`, swap).Scan(&tx); err != nil {
		t.Fatal(err)
	}
	if err := pool.QueryRow(t.Context(), `SELECT coalesce(max(actor || ' ' || reason), '') FROM swap_operator_actions
		WHERE swap_id = $1 AND action = 'choose_arrival'`, swap).Scan(&action); err != nil {
		t.Fatal(err)
	}
	return tx, action
}

func TestSwapsChooseArrival_booksTheChosenTransferAfterThePlanAndRecordsTheOperator(t *testing.T) {
	t.Parallel()
	swaps, pool, swap := chooseTool(t, "y\n", &arrivalTransfers{amount: 24_990_000, found: []bool{true}})

	code, out, stderr := runSwaps(t, swaps, "choose-arrival", swap.String(), "--tx", strings.ToUpper(arrivalHash),
		"--reason", "two transfers reached the treasury")

	if code != 0 {
		t.Fatalf("choose-arrival = %d, stderr %q", code, stderr)
	}
	wantContains(t, "stderr", stderr, "[y/N]")
	wantContains(t, "stdout", out,
		"plan swap="+swap.String(),
		"network=base arrival_tx="+arrivalHash+" arrived_micros=24990000 fee_micros=10000 changed=true",
		"booked swap="+swap.String())
	tx, action := chosenRow(t, pool, swap)
	if tx != arrivalHash || action != "operator:alice two transfers reached the treasury" {
		t.Fatalf("arrival %q and audit %q, want the chosen transfer booked by operator:alice", tx, action)
	}
}

func TestSwapsChooseArrival_aSecondRunBooksNothingAndAsksNothing(t *testing.T) {
	t.Parallel()
	swaps, pool, swap := chooseTool(t, "", &arrivalTransfers{amount: 24_990_000, found: []bool{true}})
	args := []string{"choose-arrival", swap.String(), "--tx", arrivalHash, "--reason", "first", "--yes"}
	if code, _, stderr := runSwaps(t, swaps, args...); code != 0 {
		t.Fatalf("first run = %d, stderr %q", code, stderr)
	}

	code, out, stderr := runSwaps(t, swaps, "choose-arrival", swap.String(), "--tx", arrivalHash, "--reason", "again")

	if code != 0 || strings.Contains(stderr, "[y/N]") {
		t.Fatalf("second run = %d, stderr %q, want success without a prompt", code, stderr)
	}
	wantContains(t, "stdout", out, "booked swap="+swap.String(), "changed=false")
	if _, action := chosenRow(t, pool, swap); action != "operator:alice first" {
		t.Fatalf("audit %q, want only the first run's row", action)
	}
}

func TestSwapsChooseArrival_refusesBadArgumentsAnUnknownSwapADeclinedPromptAndATransferItCannotUse(t *testing.T) {
	t.Parallel()
	swaps, pool, swap := chooseTool(t, "n\n", &arrivalTransfers{amount: 24_990_000, found: []bool{false}})
	for _, args := range [][]string{
		{"choose-arrival"},
		{"choose-arrival", "--tx", arrivalHash, "--reason", "x"},
		{"choose-arrival", "not-a-uuid", "--tx", arrivalHash, "--reason", "x"},
		{"choose-arrival", swap.String(), "--reason", "x"},
		{"choose-arrival", swap.String(), "--tx", arrivalHash},
		{"choose-arrival", swap.String(), "--tx", arrivalHash, "--reason", "x", "extra"},
	} {
		if code, _, stderr := runSwaps(t, swaps, args...); code != 2 || !strings.Contains(stderr, "choose-arrival") {
			t.Errorf("%v = %d, stderr %q, want the usage and exit 2", args, code, stderr)
		}
	}
	code, _, stderr := runSwaps(t, swaps, "choose-arrival", uuid.NewString(), "--tx", arrivalHash, "--reason", "x",
		"--yes")
	if code != 1 || !strings.Contains(stderr, "bridge_out") {
		t.Fatalf("unknown swap = %d, stderr %q, want the missing bridge_out step", code, stderr)
	}
	code, _, stderr = runSwaps(t, swaps, "choose-arrival", swap.String(), "--tx", arrivalHash, "--reason", "x", "--yes")
	if code != 1 || !strings.Contains(stderr, "not_a_cash_transfer_to_treasury") {
		t.Fatalf("wrong recipient = %d, stderr %q, want not_a_cash_transfer_to_treasury", code, stderr)
	}
	if tx, action := chosenRow(t, pool, swap); tx != "" || action != "" {
		t.Fatalf("arrival %q and audit %q after refusals, want nothing booked", tx, action)
	}
}

func TestSwapsChooseArrival_declinesAtThePromptWithoutBooking(t *testing.T) {
	t.Parallel()
	swaps, pool, swap := chooseTool(t, "n\n", &arrivalTransfers{amount: 24_990_000, found: []bool{true}})

	code, _, stderr := runSwaps(t, swaps, "choose-arrival", swap.String(), "--tx", arrivalHash, "--reason", "x")

	if code != 1 || !strings.Contains(stderr, "cancelled") {
		t.Fatalf("declined prompt = %d, stderr %q, want cancelled", code, stderr)
	}
	if tx, action := chosenRow(t, pool, swap); tx != "" || action != "" {
		t.Fatalf("arrival %q and audit %q after a declined prompt, want nothing booked", tx, action)
	}
}

func TestSwapsChooseArrival_reportsATransferThatDisappearsAfterTheConfirmation(t *testing.T) {
	t.Parallel()
	swaps, _, swap := chooseTool(t, "", &arrivalTransfers{amount: 24_990_000, found: []bool{true, false}})

	code, _, stderr := runSwaps(t, swaps, "choose-arrival", swap.String(), "--tx", arrivalHash, "--reason", "x",
		"--yes")

	if code != 1 || !strings.Contains(stderr, "not_a_cash_transfer_to_treasury") {
		t.Fatalf("choose-arrival = %d, stderr %q, want the refusal at booking time", code, stderr)
	}
}

func TestSwapsChooseArrival_reportsAMissingDatabase(t *testing.T) {
	t.Parallel()
	_, pool, identifiers, now := parkedSwapsTool(t, "")
	missing := strings.Replace(pool.Config().ConnString(), "/"+pool.Config().ConnConfig.Database,
		"/missing_"+identifiers.NewV7().String()[:8], 1)
	gone := swapsToolWith([]string{"MONACO_ENV=test", "DATABASE_URL=" + missing, "NATS_URL=" + testkit.NATSURL()},
		testkit.NewClock(now), strings.NewReader(""), fakeCashPorts(&cashBalance{micros: []uint64{1}}))

	args := []string{"choose-arrival", uuid.NewString(), "--tx", arrivalHash, "--reason", "x", "--yes"}
	if code, _, stderr := runSwaps(t, gone, args...); code != 1 || stderr == "" {
		t.Fatalf("choose-arrival on a missing database = %d, stderr %q, want 1", code, stderr)
	}
}
