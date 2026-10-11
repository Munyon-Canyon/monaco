package trading_test

import (
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (e *routeEnv) strand(t *testing.T) (app.ExecuteTrade, uuid.UUID) {
	t.Helper()
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	cmd := e.routedBuy(t)
	e.clk.Advance(app.SwapSweepAge + time.Second)
	return cmd, e.swapsOf(t, cmd.ProposalID)[0]
}

func (e *routeEnv) mustTick(t *testing.T, scanned, changed int) {
	t.Helper()
	if report, err := e.tick(t.Context(), t); err != nil || report.Scanned != scanned || report.Changed != changed {
		t.Fatalf("Tick = %+v, %v, want %d scanned and %d changed", report, err, scanned, changed)
	}
}

func TestBridgeBack_strandedCashOfAProposalNoLongerRetryableReturnsToSolana(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.mustTick(t, 1, 0)
	e.proposals.set(governance.Status("cancelled"))
	e.privy.script(multichain.ActionSucceeded)

	e.mustTick(t, 1, 1)
	e.mustTick(t, 0, 0)

	calls := e.privy.calls()
	want := multichain.TransferSpec{
		From: chain.NetworkBase, To: chain.NetworkSolana, Destination: string(e.wallet.Address),
		AmountMicros: money.MicrosFromUint64(landedMicros),
	}
	if len(calls) != 2 || calls[1].spec != want || calls[1].wallet != "evm-wallet" ||
		calls[1].key != "trade:"+swap.String()+":bridge_back" {
		t.Fatalf("transfers = %+v, want the landed cash bridged back from the EVM treasury", calls)
	}
	got := e.steps(t, swap)
	wantSteps(t, got[len(got)-3:], "bridge_back:pending:0", "bridge_back:submitted:0", "bridge_back:succeeded:7500")
}

func TestBridgeBack_aStartedReturnIsResumedEvenIfTheProposalIsRetryableAgain(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	_, swap := e.strand(t)
	e.proposals.set(governance.Status("cancelled"))
	e.mustTick(t, 1, 1)
	e.proposals.set(governance.StatusPassed)
	e.privy.script(multichain.ActionSucceeded)

	e.mustTick(t, 1, 1)

	if status, action := e.stepOf(t, swap, "bridge_back"); status != "succeeded" || action != "privy-action-2" {
		t.Fatalf("bridge_back %s %s, want succeeded on its one action", status, action)
	}
}

func TestBridgeBack_cashARetryReusedIsNeverBridgedBack(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd, first := e.strand(t)
	e.requote()
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	if code := e.retry(t, cmd, first); code != string(errs.CodeSwapFailed) {
		t.Fatalf("retry delivery %s, want swap_failed", code)
	}
	e.proposals.set(governance.Status("cancelled"))
	e.clk.Advance(app.SwapSweepAge + time.Second)

	e.mustTick(t, 1, 1)
	if err := e.exec().BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(first)); err != nil {
		t.Fatal(err)
	}

	backs := e.count(t, `SELECT count(*) FROM trade_steps WHERE step = 'bridge_back'`)
	if backs != 1 || len(e.privy.calls()) != 2 {
		t.Fatalf(
			"%d bridge_back steps after %d transfers, want the retry's cash returned once",
			backs,
			len(e.privy.calls()),
		)
	}
	if status, _ := e.stepOf(t, first, "bridge_back"); status != "" {
		t.Fatalf("the reused swap has a bridge_back step %s, want none", status)
	}
}

func TestBridgeBack_refusesWhatItCannotLoadOrJournal(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	if err := e.exec().BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(e.ids.NewV7())); err == nil {
		t.Fatal("BridgeBack of an unknown swap succeeded")
	}
	e.route.Ranked[0].Costs = []multichain.Cost{
		{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(buyMicros)},
	}
	e.privy.script(multichain.ActionSucceeded)
	cmd := e.routedBuy(t)
	if err := e.exec().
		BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(e.swapsOf(t, cmd.ProposalID)[0])); errs.CodeOf(
		err,
	) != errs.CodeInvalidInput {
		t.Fatalf("BridgeBack of nothing landed err = %v, want invalid_input", err)
	}
	e.route.Ranked[0].Costs = []multichain.Cost{
		{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(buyMicros + 1)},
	}
	big := e.request()
	if _, err := e.buy(t.Context(), big); err == nil {
		t.Fatal("a buy with a fee above the trade succeeded")
	}
	if err := e.exec().
		BridgeBack(actorContext(t.Context()), ids.SwapIDFrom(e.swapIDs(t, big.Source.ID)[0])); err == nil {
		t.Fatal("BridgeBack with a fee above the trade succeeded")
	}
}

func TestEVMTrades_reportsARetryableReadError(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.strand(t)
	e.proposals.FailOnce("Retryable", errs.New(errs.CodeUpstreamUnavailable, "governance.Retryable"))

	if report, err := e.tick(t.Context(), t); errs.CodeOf(err) != errs.CodeUpstreamUnavailable || report.Changed != 0 {
		t.Fatalf("Tick = %+v, %v, want upstream_unavailable", report, err)
	}
}

func (e *routeEnv) stepOf(t *testing.T, swap uuid.UUID, step string) (status, action string) {
	t.Helper()
	_ = e.pool.QueryRow(t.Context(), `SELECT status, coalesce(privy_action_id, '') FROM trade_steps
		WHERE trade_id = $1 AND step = $2`, swap, step).Scan(&status, &action)
	return status, action
}
