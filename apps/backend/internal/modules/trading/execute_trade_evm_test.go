package trading_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func (e *routeEnv) tradeHandler() *app.ExecuteTradeHandler {
	return app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), Routes: e.exec(), Listings: e.listings, UoW: e.uow, Reads: e.reads, Venue: e.venue,
		Ports: e.ports(), USDC: usdcToken(), EVM: true,
	})
}

func TestExecuteTrade_anEVMWinnerRunsTheRoutedBuyAndRecordsTheDelivery(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	e.quote(usdcToken(), aaplxToken(), 0, false)
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	d := e.delivery(t, cmd)

	if err := e.tradeHandler().Handle(actorContext(t.Context()), d, cmd, nil); err != nil {
		t.Fatal(err)
	}
	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 1 || e.deliveryCode(t, d) != "ok" {
		t.Fatalf("%d swaps, delivery %s, want the routed swap recorded ok", len(swaps), e.deliveryCode(t, d))
	}
	e.wantEVMRow(t, swaps[0], swapHash)
}

func TestExecuteTrade_anEVMWinnerStillChecksTheSolanaFunds(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.ledger.SetTokens(e.wallet.Address, usdcToken(), 1_000_000)
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	d := e.delivery(t, cmd)

	if err := e.tradeHandler().Handle(actorContext(t.Context()), d, cmd, nil); err != nil {
		t.Fatal(err)
	}
	if code := e.deliveryCode(t, d); code != string(errs.CodeInsufficientFunds) || len(e.privy.calls()) != 0 {
		t.Fatalf("delivery %s after %d transfers, want insufficient_funds and no bridge", code, len(e.privy.calls()))
	}
	if n := len(e.swapsOf(t, cmd.ProposalID)); n != 0 {
		t.Fatalf("%d swaps, want none", n)
	}
}
