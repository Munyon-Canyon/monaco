package trading_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func newSellSwapEnv(t *testing.T) evmSwapEnv {
	t.Helper()
	e := newEVMSwapEnv(t)
	swap := e.evm.plan.Swap
	e.evm.plan = app.EVMSwapPlan{Approvals: e.evm.plan.Approvals, Candidates: []app.EVMSwapCandidate{
		{Swap: swap, OutAmount: 310},
		{Route: multichain.RouteOptions{HooksOptions: multichain.NoHooks}, Swap: swap, OutAmount: 305},
	}}
	e.evm.chosen = app.EVMSwapPlan{Swap: swap, OutAmount: 306, MinOut: 303}
	e.leg.Spec.MinOut, e.leg.Spec.SlippageBps = 0, 100
	return e
}

func TestEVMSwap_aSellSendsItsApprovalsOnceBeforeChoosingARoute(t *testing.T) {
	t.Parallel()
	e := newSellSwapEnv(t)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: 306}}

	if got, err := e.run(t.Context()); err != nil || got.State != app.FillSucceeded {
		t.Fatalf("Run = %+v, %v, want the fill", got, err)
	}
	want := []string{"send:" + e.key() + ":approve:0", "receipt:0xhash1", "choose", "send:" + e.key()}
	if got := e.evm.steps(); !slices.Equal(got, want) {
		t.Fatalf("steps = %v, want %v", got, want)
	}
	if len(e.evm.choices) != 1 || len(e.evm.choices[0]) != 2 ||
		e.evm.choices[0][1].Route.HooksOptions != multichain.NoHooks {
		t.Fatalf("choices = %+v, want the built candidates once", e.evm.choices)
	}
	e.wantSubmittedOnce(t)
}

func TestEVMSwap_aSellWithNoRouteThatSimulatesSendsNoSwap(t *testing.T) {
	t.Parallel()
	for name, fail := range map[string]func(e evmSwapEnv){
		"no route":     func(e evmSwapEnv) { e.evm.chooseErr = errs.New(errs.CodeNoRoute, "evm.Choose") },
		"under floor":  func(e evmSwapEnv) { e.evm.chosen.MinOut = 301 },
		"rpc down":     func(e evmSwapEnv) { e.evm.chooseErr = errs.New(errs.CodeRPCUnavailable, "evm.Choose") },
		"empty choice": func(e evmSwapEnv) { e.evm.chosen = app.EVMSwapPlan{} },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newSellSwapEnv(t)
			fail(e)

			if _, err := e.run(t.Context()); err == nil {
				t.Fatal("Run returned no error")
			}
			if keys := e.evm.keys(); !slices.Equal(keys, []string{e.key() + ":approve:0"}) {
				t.Fatalf("sent %v, want only the approval", keys)
			}
			e.wantUnsent(t)
		})
	}
}

func TestEVMSwap_aBuyNeverAsksTheVenueToChoose(t *testing.T) {
	t.Parallel()
	e := newEVMSwapEnv(t)
	e.leg.Spec.Route = multichain.RouteOptions{HooksOptions: multichain.NoHooks}
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: 306}}

	if _, err := e.run(t.Context()); err != nil {
		t.Fatal(err)
	}
	if len(e.evm.choices) != 0 || e.evm.builds != 1 || e.evm.specs[0].Route.HooksOptions != multichain.NoHooks ||
		e.evm.specs[0].TokenOut != evmAddress(t, baseToken) {
		t.Fatalf("choices %v after %d builds of %+v, want one build on the quoted route and no choice",
			e.evm.choices, e.evm.builds, e.evm.specs)
	}
}
