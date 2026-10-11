package treasury_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func evmConfirmed(cabal ids.CabalID, swap uuid.UUID, units uint64) events.TradeConfirmed {
	const landed, fee = 9_992_500, 7_500
	return events.TradeConfirmed{
		V: 1, SwapID: swap, CabalID: cabal.UUID(), Action: "buy", Symbol: "AAPLx",
		InMint: baseCash, InAmount: landed, OutMint: baseAAPL, OutAmount: units,
		USDCMicros: money.MicrosFromUint64(landed + fee), FeeMicros: money.MicrosFromUint64(fee),
		TxSignature: "0xswap", ConfirmedAt: confirmedAt(),
	}
}

func (f fixture) feesBalance(t *testing.T, cabal ids.CabalID, asset string) int64 {
	t.Helper()
	var sum int64
	if err := f.pool.QueryRow(t.Context(), `SELECT coalesce(sum(e.amount), 0)::bigint
		FROM cabal_txn_entries e JOIN cabal_txns c ON c.id = e.txn_id
		WHERE c.cabal_id = $1 AND e.account = 'fees' AND e.asset = $2`, cabal.UUID(), asset).Scan(&sum); err != nil {
		t.Fatal(err)
	}
	return sum
}

func (f fixture) wantClean(t *testing.T, at string) {
	t.Helper()
	if drift := f.drift(t); len(drift) != 0 {
		t.Fatalf("%s: drift = %q", at, drift)
	}
}

func TestBridgeFee_aBridgedBuyCostsTheSolanaUSDCSentAndNoCashMovesTwice(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	out := newBridgeRun(t, f, cabal, "bridge_out")
	out.submitted(t, 10_000_000)
	out.succeeded(t, 10_000_000, 7_500)

	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "90000000", Cost: "90000000"},
		position{Asset: baseCash, Units: "9992500", Cost: "9992500"},
	)
	if fees := f.feesBalance(t, cabal, baseCash); fees != 7_500 {
		t.Fatalf("fees on %s = %d after the landing, want the 7500 bridge fee", baseCash, fees)
	}
	f.wantClean(t, "bridge landed")

	if err := f.confirm(t, evmConfirmed(cabal, out.swap, 400_000_000)); err != nil {
		t.Fatal(err)
	}

	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "90000000", Cost: "90000000"},
		position{Asset: baseCash, Units: "0", Cost: "0"},
		position{Asset: baseAAPL, Units: "400000000", Cost: "10000000"},
	)
	if fees, usdc := f.feesBalance(t, cabal, baseCash), f.feesBalance(t, cabal, usdcMint); fees != 0 || usdc != 0 {
		t.Fatalf("fees %d %s and %d USDC, want the bridge fee capitalized and no Solana fee", fees, baseCash, usdc)
	}
	f.wantClean(t, "swap")
}

func TestBridgeFee_strandedCashKeepsItsFeeAsAnExpenseUntilARetryCapitalizesIt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	f.bridgedOut(t, cabal, 10_000_000, 7_500)
	f.wantClean(t, "stranded")

	retry := f.ids.NewV7()
	if err := f.confirm(t, evmConfirmed(cabal, retry, 390_000_000)); err != nil {
		t.Fatal(err)
	}

	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "90000000", Cost: "90000000"},
		position{Asset: baseCash, Units: "0", Cost: "0"},
		position{Asset: baseAAPL, Units: "390000000", Cost: "10000000"},
	)
	f.wantClean(t, "retry")
}

func TestBridgeFee_bridgingStrandedCashBackRealizesBothFees(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	f.bridgedOut(t, cabal, 10_000_000, 7_500)
	back := newBridgeRun(t, f, cabal, "bridge_back")

	back.submitted(t, 9_992_500)
	f.wantClean(t, "bridge back in transit")
	back.succeeded(t, 9_992_500, 7_494)

	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "99985006", Cost: "99985006"},
		position{Asset: baseCash, Units: "0", Cost: "0"},
	)
	if out, home := f.feesBalance(
		t,
		cabal,
		baseCash,
	), f.feesBalance(
		t,
		cabal,
		usdcMint,
	); out != 7_500 ||
		home != 7_494 {
		t.Fatalf("fees = %d out and %d back, want both bridge fees on the fees account", out, home)
	}
	f.wantClean(t, "bridge back landed")
}

func TestBridgeFee_theCostCheckStillCatchesPlantedDrift(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	out := newBridgeRun(t, f, cabal, "bridge_out")
	out.submitted(t, 10_000_000)
	out.succeeded(t, 10_000_000, 7_500)
	if err := f.confirm(t, evmConfirmed(cabal, out.swap, 400_000_000)); err != nil {
		t.Fatal(err)
	}
	if _, err := f.pool.Exec(t.Context(), `UPDATE cabal_positions SET cost_basis_micros = 9992500
		WHERE cabal_id = $1 AND asset = $2`, cabal.UUID(), baseAAPL); err != nil {
		t.Fatal(err)
	}

	if drift := f.drift(t); len(drift) != 1 {
		t.Fatalf("drift = %q, want the planted basis of the landed amount reported", drift)
	}
}

func TestTrades_linksTheBoughtPositionToItsListing(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	evm := f.ids.NewV7()
	var solana uuid.UUID
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO asset_listings
		(id, issuer, symbol, network, chain_id, token_address, decimals, shares_per_token, venue, enabled, created_at)
		VALUES ($1, 'coinbase', 'AAPLc', 'base', 8453, $2, 8, 1, 'uniswap', false, now())`,
		evm, baseAAPL[len("base:"):]); err != nil {
		t.Fatal(err)
	}
	if err := f.pool.QueryRow(t.Context(), `SELECT id FROM asset_listings WHERE network = 'solana'
		AND token_address = $1`, aapl).Scan(&solana); err != nil {
		t.Fatal(err)
	}
	f.bridgedOut(t, cabal, 10_000_000, 7_500)

	if err := f.confirm(t, evmConfirmed(cabal, f.ids.NewV7(), 400_000_000)); err != nil {
		t.Fatal(err)
	}
	if err := f.confirm(t, buy(cabal, f.ids.NewV7(), 5_000_000, 21_000_000, 0)); err != nil {
		t.Fatal(err)
	}

	for asset, want := range map[string]uuid.UUID{baseAAPL: evm, string(aapl): solana} {
		var got uuid.UUID
		if err := f.pool.QueryRow(t.Context(), `SELECT listing_id FROM cabal_positions
			WHERE cabal_id = $1 AND asset = $2`, cabal.UUID(), asset).Scan(&got); err != nil || got != want {
			t.Fatalf("%s listing = %s, %v, want %s", asset, got, err, want)
		}
	}
}
