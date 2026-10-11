//go:build faultpoints

package trading_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (e *routeEnv) crashBuy(t *testing.T, point faultpoint.Name, after int) uuid.UUID {
	t.Helper()
	req := e.request()
	ctx := faultpoint.ArmedAfter(actorContext(t.Context()), point, after)
	crashed := func() (p any) {
		defer func() { p = recover() }()
		_, _ = e.exec().Buy(ctx, req, e.listing, nil)
		return nil
	}()
	if crashed != (faultpoint.Crash{Name: point}) {
		t.Fatalf("the buy ended with %v, want a crash at %s", crashed, point)
	}
	swaps := e.swapIDs(t, req.Source.ID)
	if len(swaps) != 1 {
		t.Fatalf("%d swaps after the crash, want 1", len(swaps))
	}
	return swaps[0]
}

func TestRouteExec_aCrashAtEveryStepBoundaryResumesWithOneBridgeAndOneSwap(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		point faultpoint.Name
		after int
	}{
		"after the bridge transfer":      {faultpoint.AfterBroadcast, 0},
		"after the bridge landed":        {faultpoint.AfterExecute, 0},
		"after the bridge was journaled": {faultpoint.AfterBridgeLanded, 0},
		"after the requote":              {faultpoint.AfterRequote, 0},
		"after the swap send":            {faultpoint.AfterBroadcast, 1},
		"after the swap was stored":      {faultpoint.AfterSwapSubmitted, 0},
		"after the fill":                 {faultpoint.AfterExecute, 1},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			e.requote()
			e.privy.script(multichain.ActionSucceeded, multichain.ActionSucceeded)
			fill := app.EVMFill{State: app.FillSucceeded, Out: filledOut}
			e.evm.fills = []app.EVMFill{fill, fill}

			swap := e.crashBuy(t, tc.point, tc.after)
			e.resume(t, ids.SwapIDFrom(swap), domain.StatusConfirmed)
			e.resume(t, ids.SwapIDFrom(swap), domain.StatusConfirmed)

			if e.privy.actions() != 1 || e.evm.hashes() != 2 {
				t.Fatalf("%d bridge actions and %d sent transactions, want one bridge and one approval plus one swap",
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

func TestRouteExec_aCrashRightAfterTheCreateLeavesTheSwapToTheSweeper(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)

	swap := e.crashBuy(t, faultpoint.AfterCreate, 0)
	e.clk.Advance(app.SwapSweepAge + time.Second)
	sweeper := app.NewSwapSweeper(e.uow, e.pool, e.clk, nil, e.hints, app.DefaultSweepTiming())
	if _, err := sweeper.Tick(actorContext(t.Context())); err != nil {
		t.Fatal(err)
	}

	if status, _, _, _ := e.row(t, swap); status != string(domain.StatusFailed) || len(e.privy.calls()) != 0 {
		t.Fatalf("swap %s after %d transfers, want failed by the sweeper with nothing bridged",
			status, len(e.privy.calls()))
	}
}
