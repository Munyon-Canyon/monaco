package treasury_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (f fixture) txnCount(t *testing.T, cabal ids.CabalID) int {
	t.Helper()
	var n int
	if err := f.pool.QueryRow(t.Context(), `SELECT count(*) FROM cabal_txns WHERE cabal_id = $1`,
		cabal.UUID()).Scan(&n); err != nil {
		t.Fatal(err)
	}
	return n
}

func TestBridgeAdjust_aLateArrivalMovesTheFeeDifferenceOnceAndTheSwapSpendsIt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	out := newBridgeRun(t, f, cabal, "bridge_out")
	out.submitted(t, 1_000_000)
	out.succeeded(t, 1_000_000, 25_930)
	before := f.txnCount(t, cabal)

	for range 2 {
		out.succeeded(t, 1_000_000, 27_493)
	}

	if n := f.txnCount(t, cabal) - before; n != 1 {
		t.Fatalf("%d txns posted for the arrival, want one adjustment", n)
	}
	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "99000000", Cost: "99000000"},
		position{Asset: baseCash, Units: "972507", Cost: "972507"},
	)
	if fees := f.feesBalance(t, cabal, baseCash); fees != 27_493 {
		t.Fatalf("fees = %d, want the real 27493 bridge fee", fees)
	}
	f.wantClean(t, "adjusted")

	confirmed := evmConfirmed(cabal, out.swap, 400_000_000)
	confirmed.InAmount, confirmed.FeeMicros = 972_507, money.MicrosFromUint64(27_493)
	if err := f.confirm(t, confirmed); err != nil {
		t.Fatal(err)
	}
	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "99000000", Cost: "99000000"},
		position{Asset: baseCash, Units: "0", Cost: "0"},
		position{Asset: baseAAPL, Units: "400000000", Cost: "1000000"},
	)
	f.wantClean(t, "swap")
}

func TestBridgeAdjust_aCheaperBridgeMovesTheDifferenceBackToTheTreasury(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	out := newBridgeRun(t, f, cabal, "bridge_out")
	out.submitted(t, 1_000_000)
	out.succeeded(t, 1_000_000, 25_930)

	out.succeeded(t, 1_000_000, 20_000)

	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "99000000", Cost: "99000000"},
		position{Asset: baseCash, Units: "980000", Cost: "980000"},
	)
	if fees := f.feesBalance(t, cabal, baseCash); fees != 20_000 {
		t.Fatalf("fees = %d, want the real 20000 bridge fee", fees)
	}
	f.wantClean(t, "adjusted down")
}

func TestBridgeAdjust_aReusedBridgeOutAdjustsTheSwapThatBookedIt(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	first := newBridgeRun(t, f, cabal, "bridge_out")
	first.submitted(t, 1_000_000)
	first.succeeded(t, 1_000_000, 25_930)
	retry := newBridgeRun(t, f, cabal, "bridge_out")
	reused := retry.event("succeeded", "reused:"+first.swap.String(), 1_000_000, 27_493)

	for range 2 {
		retry.handle(t, reused)
	}

	wantInTransit(t, f, cabal)
	wantBridgePositions(t, f,
		position{Asset: usdcMint, Units: "99000000", Cost: "99000000"},
		position{Asset: baseCash, Units: "972507", Cost: "972507"},
	)
	confirmed := evmConfirmed(cabal, retry.swap, 400_000_000)
	confirmed.InAmount, confirmed.FeeMicros = 972_507, money.MicrosFromUint64(27_493)
	if err := f.confirm(t, confirmed); err != nil {
		t.Fatal(err)
	}
	if units := f.cabalPositions(t)[1]; units.Units != "0" || f.feesBalance(t, cabal, baseCash) != 0 {
		t.Fatalf("EVM cash %+v and fees %d after the retry, want both 0", units, f.feesBalance(t, cabal, baseCash))
	}
	f.wantClean(t, "retry")
}

func TestBridgeAdjust_aReusedStepNamingNoSwapIsRefused(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	retry := newBridgeRun(t, f, cabal, "bridge_out")

	err := f.stepChanged(retry.event("succeeded", "reused:"+uuid.Nil.String()[:8], 1_000_000, 27_493))

	wantCode(t, err, errs.CodeInvalidInput)
}
