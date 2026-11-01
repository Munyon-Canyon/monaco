package treasury_test

import (
	"strconv"
	"testing"
)

const bridgedMicros = 10_000_000

func (r bridgeRun) submittedWithEstimate(t *testing.T, estimate uint64) {
	t.Helper()
	r.handle(t, r.event("pending", "", bridgedMicros, 0))
	r.handle(t, r.event("submitted", "privy-"+r.swap.String(), bridgedMicros, estimate))
}

func TestTradeSteps_inTransitDuringABridgeIsSentLessTheQuotedFee(t *testing.T) {
	t.Parallel()
	for _, step := range []string{"bridge_out", "bridge_back"} {
		t.Run(step, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			cabal := f.funded(t)
			if step == "bridge_back" {
				f.bridgedOut(t, cabal, 20_000_000, 0)
			}
			run := newBridgeRun(t, f, cabal, step)

			run.submittedWithEstimate(t, 25_000)

			wantInTransit(t, f, cabal, inTransit{Swap: run.swap, Network: "base", Micros: "9975000"})
		})
	}
}

func TestTradeSteps_aZeroQuotedFeeLeavesInTransitAtTheFullSentAmount(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	out := newBridgeRun(t, f, cabal, "bridge_out")

	out.submittedWithEstimate(t, 0)

	wantInTransit(t, f, cabal, inTransit{Swap: out.swap, Network: "base", Micros: "10000000"})
}

func TestTradeSteps_aQuotedFeeAtOrAboveTheSentAmountFloorsInTransitAtNothing(t *testing.T) {
	t.Parallel()
	for _, estimate := range []uint64{10_000_000, 12_000_000} {
		t.Run(strconv.FormatUint(estimate, 10), func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			cabal := f.funded(t)
			out := newBridgeRun(t, f, cabal, "bridge_out")

			out.submittedWithEstimate(t, estimate)

			wantInTransit(t, f, cabal)
			wantBridgePositions(t, f, position{Asset: usdcMint, Units: "90000000", Cost: "90000000"})
			out.succeeded(t, bridgedMicros, 7_500)
			wantInTransit(t, f, cabal)
			wantBridgePositions(t, f,
				position{Asset: usdcMint, Units: "90000000", Cost: "90000000"},
				position{Asset: baseCash, Units: "9992500", Cost: "9992500"},
			)
		})
	}
}

func TestTradeSteps_theBooksMatchTheArrivedAmountWhateverTheQuotedFeeWas(t *testing.T) {
	t.Parallel()
	const realFee = 7_500
	for name, estimate := range map[string]uint64{"above": 25_000, "equal": realFee, "below": 1_000, "zero": 0} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			f := newFixture(t)
			cabal := f.funded(t)
			out := newBridgeRun(t, f, cabal, "bridge_out")

			out.submittedWithEstimate(t, estimate)
			out.succeeded(t, bridgedMicros, realFee)

			wantInTransit(t, f, cabal)
			wantBridgePositions(t, f,
				position{Asset: usdcMint, Units: "90000000", Cost: "90000000"},
				position{Asset: baseCash, Units: "9992500", Cost: "9992500"},
			)
			if drift := f.drift(t); len(drift) != 0 {
				t.Fatalf("drift = %q", drift)
			}
		})
	}
}

func TestTradeSteps_aBridgeThatFailsAfterAQuotedSubmitRestoresTheFullAmount(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	cabal := f.funded(t)
	out := newBridgeRun(t, f, cabal, "bridge_out")

	out.submittedWithEstimate(t, 25_000)
	out.failed(t, bridgedMicros)

	wantInTransit(t, f, cabal)
	wantBridgePositions(t, f, position{Asset: usdcMint, Units: "100000000", Cost: "100000000"})
}
