package trading_test

import (
	"math/big"
	"strconv"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	mcfakes "github.com/monaco/monaco/apps/backend/internal/testkit/fakes/multichain"
)

const hookless = "V4_NO_HOOKS"

func (e *routeEnv) requoteWith(q multichain.Quote) {
	e.quotes = mcfakes.New().Quoter
	e.quotes.Quotes.Push(mcfakes.Reply[multichain.Quote]{Value: q})
}

func TestRouteExec_aSubmittedBuyResumesWithoutABuildOrARequote(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.requoteWith(multichain.Quote{
		GrossOutput: big.NewInt(requotedOut), Options: multichain.RouteOptions{HooksOptions: hookless},
	})
	e.privy.script(multichain.ActionSucceeded)
	view := e.mustBuy(t, e.request(), domain.StatusSubmitted)

	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	e.resume(t, view.ID, domain.StatusConfirmed)

	if e.evm.builds != 1 || e.quotes.Quotes.Calls() != 1 {
		t.Fatalf("%d builds after %d requotes, want the resume to only await the submitted swap",
			e.evm.builds, e.quotes.Quotes.Calls())
	}
	if got := e.evm.specs[0].Route.HooksOptions; got != hookless {
		t.Fatalf("the swap was built with hooks %q, want the route the requote chose", got)
	}
}

func TestRouteExec_theSwapFloorIsTheSlippageFloorOfTheSimulatedOutput(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	const simulated, floor = 300_000_000, 297_000_000
	e.requoteWith(multichain.Quote{GrossOutput: big.NewInt(simulated)})
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	e.mustBuy(t, e.request(), domain.StatusConfirmed)

	if got := e.evm.specs[0].MinOut; got != floor {
		t.Fatalf("swap MinOut = %d, want 1%% under the simulated %d", got, simulated)
	}
}

func TestRetry_aStrandedRetryBuysOnSolanaWithTheObservedBridgeBackArrival(t *testing.T) {
	t.Parallel()
	r := newStrandedRig(t)
	r.privy.script(multichain.ActionSucceeded)
	const observedFee = bridgeFeeMicros + 1_234
	r.arrivals.short = observedFee
	onEVM := uint64(buyMicros - bridgeFeeMicros)
	arrived := onEVM - observedFee

	code := r.retryThrough(t, r.handler(r.router(900_000, 1_000_000)), r.cmd, r.first)

	second := r.retrySwap(t, r.cmd.ProposalID)
	if in := r.swapIn(t, second); code != "ok" || in != arrived {
		t.Fatalf("retry %s bought on Solana with %d, want the %d that arrived", code, in, arrived)
	}
	got := r.steps(t, r.first)
	if want := "bridge_back:succeeded:" + strconv.Itoa(observedFee); got[len(got)-1] != want {
		t.Fatalf("stranded swap steps = %v, want the bridge back to record the observed fee as %s", got, want)
	}
	cash, err := r.exec().Stranded(actorContext(t.Context()), ids.SwapIDFrom(r.first))
	if err != nil || cash.OnEVM.Uint64() != onEVM || cash.Back.Uint64() != arrived {
		t.Fatalf("Stranded = %+v, %v, want %d back as observed", cash, err, arrived)
	}
}

func TestRetry_aBridgeBackWhoseArrivalCannotBeReadStaysSubmittedUntilItCan(t *testing.T) {
	t.Parallel()
	cases := map[string]func(r *strandedRig){
		"no Solana step yet": func(r *strandedRig) { r.privy.noArrival = true },
		"the arrival read fails": func(r *strandedRig) {
			r.arrivals.err = errs.New(errs.CodeRPCUnavailable, "solana.InboundTransfers")
		},
	}
	for name, unreadable := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newStrandedRig(t)
			r.privy.script(multichain.ActionSucceeded)
			unreadable(r)
			h := r.handler(r.router(900_000, 1_000_000))

			if _, err := r.retryHandle(t, h, r.cmd, r.first); !errs.Retryable(errs.CodeOf(err)) {
				t.Fatalf("first delivery err = %v, want a retryable error", err)
			}
			if got := r.steps(t, r.first); got[len(got)-1] != "bridge_back:submitted:7500" {
				t.Fatalf("stranded swap steps = %v, want the bridge back left submitted", got)
			}
			r.privy.noArrival = false
			code := r.retryThrough(t, h, r.cmd, r.first)

			swaps := r.swapsOf(t, r.cmd.ProposalID)
			if in := r.swapIn(t, swaps[len(swaps)-1]); code != "ok" || r.bridgeBacks() != 1 ||
				in != buyMicros-2*bridgeFeeMicros {
				t.Fatalf("redelivery %s after %d bridge backs bought %d, want one bridge back and the arrival bought",
					code, r.bridgeBacks(), in)
			}
		})
	}
}

func TestRouteExec_strandedCashFailsWhenTheLandedBridgeBackCannotBeRead(t *testing.T) {
	t.Parallel()
	r := newStrandedRig(t)
	r.privy.script(multichain.ActionSucceeded)
	r.retryThrough(t, r.handler(r.router(900_000, 1_000_000)), r.cmd, r.first)
	r.privy.actionErr = errs.New(errs.CodeUpstreamUnavailable, "privy.Action")

	if _, err := r.exec().Stranded(actorContext(t.Context()), ids.SwapIDFrom(r.first)); err == nil {
		t.Fatal("Stranded read the landed bridge back without its Privy action")
	}
}

func TestRetry_anArrivalAboveWhatWasSentRecordsNoFee(t *testing.T) {
	t.Parallel()
	r := newStrandedRig(t)
	r.privy.script(multichain.ActionSucceeded)
	r.arrivals.short, r.arrivals.over = 0, 5
	onEVM := uint64(buyMicros - bridgeFeeMicros)

	code := r.retryThrough(t, r.handler(r.router(900_000, 1_000_000)), r.cmd, r.first)

	got := r.steps(t, r.first)
	second := r.retrySwap(t, r.cmd.ProposalID)
	if in := r.swapIn(t, second); code != "ok" || got[len(got)-1] != "bridge_back:succeeded:0" || in != onEVM {
		t.Fatalf("retry %s, steps %v, Solana buy of %d, want no fee and at most the %d sent", code, got, in, onEVM)
	}
}
