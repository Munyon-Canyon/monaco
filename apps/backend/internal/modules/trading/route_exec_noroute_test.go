package trading_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	mcfakes "github.com/monaco/monaco/apps/backend/internal/testkit/fakes/multichain"
)

func TestRouteExec_aResumedSwapWhoseRequoteFindsNoRouteFailsItsStepSoTheCashIsSwept(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.buildErr = errs.New(errs.CodeUpstreamUnavailable, "uniswap.Build")
	req := e.request()
	if _, err := e.buy(t.Context(), req); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Buy err = %v, want the build outage", err)
	}
	swap := ids.SwapIDFrom(e.swapIDs(t, req.Source.ID)[0])
	if step, started, err := e.journal.Step(t.Context(), swap, domain.StepSwap); err != nil || !started ||
		step.Status == domain.StepFailed || step.Status == domain.StepSucceeded {
		t.Fatalf("swap step = %+v started %v, %v, want it open after the outage", step, started, err)
	}

	e.quotes = mcfakes.New().Quoter
	e.quotes.Quotes.Push(mcfakes.Reply[multichain.Quote]{Code: errs.CodeNoRoute})
	e.resume(t, swap, domain.StatusFailed)

	step, _, err := e.journal.Step(t.Context(), swap, domain.StepSwap)
	if err != nil || step.Status != domain.StepFailed || step.Error != string(domain.FailurePriceMoved) {
		t.Fatalf("swap step = %+v, %v, want failed with price_moved", step, err)
	}
	if code := e.failure(t, swap.UUID()); code != string(domain.FailurePriceMoved) {
		t.Fatalf("failure = %s, want price_moved", code)
	}
	if n := len(e.privy.calls()); n != 1 {
		t.Fatalf("%d transfers, want the one bridge and the cash left on the EVM treasury", n)
	}

	e.clk.Advance(app.SwapSweepAge + time.Second)
	e.mustTick(t, 1, 0)
	e.proposals.set(governance.Status("cancelled"))
	e.privy.script(multichain.ActionSucceeded)
	e.mustTick(t, 1, 1)
	if status, _ := e.bridgeBackOf(t, swap.UUID()); status != "succeeded" {
		t.Fatalf("bridge_back %s, want the stranded cash swept back to Solana", status)
	}
}
