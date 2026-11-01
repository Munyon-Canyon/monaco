package trading_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func TestExecuteTrade_aRedeliveryAfterAFailedRoutedBuyDoesNotBridgeAgain(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	e.quote(usdcToken(), aaplxToken(), 0, false)
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	first := e.delivery(t, cmd)
	if err := e.tradeHandler().Handle(actorContext(t.Context()), first, cmd, nil); err != nil {
		t.Fatal(err)
	}
	bridges := len(e.privy.calls())
	if bridges == 0 || e.deliveryCode(t, first) != string(errs.CodeSwapFailed) {
		t.Fatalf("%d transfers, delivery %s, want a bridged buy that failed", bridges, e.deliveryCode(t, first))
	}

	again := e.delivery(t, cmd)
	if err := e.tradeHandler().Handle(actorContext(t.Context()), again, cmd, nil); err != nil {
		t.Fatal(err)
	}
	if n := len(e.privy.calls()); n != bridges {
		t.Fatalf("%d transfers after the redelivery, want the %d from the first run", n, bridges)
	}
	if n := len(e.swapsOf(t, cmd.ProposalID)); n != 1 {
		t.Fatalf("%d swaps after the redelivery, want 1", n)
	}
	if code := e.deliveryCode(t, again); code != string(errs.CodeSwapFailed) {
		t.Fatalf("redelivery recorded %s, want swap_failed", code)
	}
}

func TestExecuteTrade_aRedeliveryThatCannotReadTheBridgeStepIsRetried(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	e.quote(usdcToken(), aaplxToken(), 0, false)
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	if err := e.tradeHandler().Handle(actorContext(t.Context()), e.delivery(t, cmd), cmd, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := e.pool.Exec(t.Context(), `ALTER TABLE trade_steps RENAME TO trade_steps_gone`); err != nil {
		t.Fatal(err)
	}
	again := e.delivery(t, cmd)

	err := e.tradeHandler().Handle(actorContext(t.Context()), again, cmd, nil)

	if errs.CodeOf(err) != errs.CodeInternal || e.recorded(t, again) {
		t.Fatalf(
			"Handle err = %v, recorded %v, want the unreadable step returned for a retry",
			err,
			e.recorded(t, again),
		)
	}
}
