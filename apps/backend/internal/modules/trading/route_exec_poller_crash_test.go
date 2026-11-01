//go:build faultpoints

package trading_test

import (
	"testing"
	"time"

	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func TestEVMTrades_aBuyCrashedAtAnyStepFinishesThroughThePoller(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		point faultpoint.Name
		after int
	}{
		"bridge submit":  {faultpoint.AfterBroadcast, 0},
		"bridge landed":  {faultpoint.AfterExecute, 0},
		"bridge journal": {faultpoint.AfterBridgeLanded, 0},
		"requote":        {faultpoint.AfterRequote, 0},
		"swap send":      {faultpoint.AfterBroadcast, 1},
		"swap submitted": {faultpoint.AfterSwapSubmitted, 0},
		"fill":           {faultpoint.AfterExecute, 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			e.requote()
			e.privy.script(multichain.ActionSucceeded, multichain.ActionSucceeded)
			fill := app.EVMFill{State: app.FillSucceeded, Out: filledOut}
			e.evm.fills = []app.EVMFill{fill, fill}

			swap := e.crashBuy(t, tc.point, tc.after)
			e.clk.Advance(app.SwapSweepAge + time.Second)
			e.mustTick(t, 1, 1)
			e.mustTick(t, 0, 0)

			if status, _, _, _ := e.row(t, swap); status != string(domain.StatusConfirmed) {
				t.Fatalf("swap is %s after the poller, want confirmed", status)
			}
			if e.privy.actions() != 1 || e.evm.hashes() != 2 {
				t.Fatalf("%d bridge actions and %d sent transactions, want one bridge, one approval and one swap",
					e.privy.actions(), e.evm.hashes())
			}
			if n := e.count(t, `SELECT count(*) FROM events WHERE type = 'trade.confirmed'
				AND aggregate_id = $1`, swap); n != 1 {
				t.Fatalf("%d trade.confirmed, want 1", n)
			}
			wantSteps(t, e.steps(t, swap), append(bridged(),
				"swap:pending:0", "swap:submitted:0", "swap:succeeded:0")...)
			e.wantEVMRow(t, swap, swapHash)
		})
	}
}

func TestEVMTrades_aBridgeBackCrashedAfterItsTransferFinishesWithOneAction(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.proposals.set(governance.Status("cancelled"))
	e.privy.script(multichain.ActionSucceeded)
	armed := faultpoint.Armed(actorContext(t.Context()), faultpoint.AfterBroadcast)
	crashed := func() (p any) {
		defer func() { p = recover() }()
		_, _ = e.poller().Tick(armed)
		return nil
	}()
	if crashed != (faultpoint.Crash{Name: faultpoint.AfterBroadcast}) {
		t.Fatalf("the tick ended with %v, want a crash after the bridge back transfer", crashed)
	}

	e.mustTick(t, 1, 1)
	e.mustTick(t, 0, 0)

	if status, action := e.bridgeBackOf(t, swap); status != "succeeded" || e.privy.actions() != 2 ||
		action != "privy-action-2" {
		t.Fatalf("bridge_back %s %s after %d actions, want one bridge out and one bridge back",
			status, action, e.privy.actions())
	}
	got := e.steps(t, swap)
	wantSteps(t, got[len(got)-3:], "bridge_back:pending:0", "bridge_back:submitted:7500", "bridge_back:succeeded:7500")
}
