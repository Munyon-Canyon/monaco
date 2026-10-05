package main

import (
	"bytes"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestFormatUSDC(t *testing.T) {
	t.Parallel()
	for raw, want := range map[uint64]string{0: "0.000000 USDC", 1_500_000: "1.500000 USDC", 44765: "0.044765 USDC"} {
		if got := formatUSDC(money.NewBaseUnits(raw, 6)); got != want {
			t.Fatalf("formatUSDC(%d) = %q, want %q", raw, got, want)
		}
	}
}

func TestFormatAddress(t *testing.T) {
	t.Parallel()
	if got := formatAddress("EfFBEMVogFPxpTNvfVFxdYc89KojsRNDzHDuuoKwqtrF"); got != "EfFB…qtrF" {
		t.Fatalf("formatAddress() = %q", got)
	}
	if got := formatAddress("short"); got != "short" {
		t.Fatalf("formatAddress(short) = %q", got)
	}
}

func TestPrintSweepRecap_dryRunTotalsWhatWouldMove(t *testing.T) {
	t.Parallel()
	var out bytes.Buffer
	printSweepRecap(&out, dest, true, []walletOutcome{{
		source: sweepSource{kindMember, member}, balance: money.NewBaseUnits(1_250_000, 6),
		status: statusDryRun, note: "no tx sent",
	}})
	for _, want := range []string{
		"=== Sweep summary (dry-run) ===",
		"dry-run usdc-sweep amount=1.250000 USDC (1250000 raw) (no tx sent)",
		"Would sweep: 1.250000 USDC across 1 wallet(s)",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("recap missing %q\n%s", want, out.String())
		}
	}
	out.Reset()
	printSweepRecap(&out, dest, true, nil)
	if !strings.Contains(out.String(), "No wallets.") {
		t.Fatalf("empty recap = %s", out.String())
	}
}

func TestTally_exitsOneOnlyWhenAWalletFailed(t *testing.T) {
	t.Parallel()
	if (tally{ok: 2, skipped: 1}).exitCode() != 0 || (tally{ok: 2, failed: 1}).exitCode() != 1 {
		t.Fatal("exit code must be 1 exactly when a wallet failed")
	}
}

func TestPrintSweepRecap_totalsWhatMovedAndCountsEachStatus(t *testing.T) {
	t.Parallel()
	usd := func(v uint64) money.BaseUnits { return money.NewBaseUnits(v, 6) }
	outcomes := []walletOutcome{
		{
			source: sweepSource{kindMember, member}, balance: usd(2_000_000), status: statusOK,
			tx: "5kLmNopQrStUvWxYzAbCdEfGhIjKlMnOpQrStUvWxYzAbCdEfGh",
		},
		{source: sweepSource{kindTreasury, cabal}, balance: usd(500_000), status: statusFail, note: "build: boom"},
		{source: sweepSource{kindPrivy, stray}, balance: usd(0), status: statusSkipped, note: "zero USDC"},
		{source: sweepSource{kindMember, stray}, balance: usd(1_000_000), status: statusOK, tx: "6mNoPqRsTuVwXyZa"},
	}
	var out bytes.Buffer
	printSweepRecap(&out, dest, false, outcomes)
	for _, want := range []string{
		"=== Sweep summary (live) ===",
		"Wallets: 4 (1 FAIL)",
		"ok usdc-sweep amount=2.000000 USDC (2000000 raw) tx=5kLm…EfGh",
		"FAIL usdc-sweep amount=0.500000 USDC (500000 raw) err=build: boom",
		"skipped usdc-sweep amount=0.000000 USDC (0 raw) (zero USDC)",
		"Swept: 3.000000 USDC across 2 wallet(s)",
		"Wallets: ok=2 fail=1 skipped=1 dry-run=0",
	} {
		if !strings.Contains(out.String(), want) {
			t.Fatalf("recap missing %q\n%s", want, out.String())
		}
	}
}
