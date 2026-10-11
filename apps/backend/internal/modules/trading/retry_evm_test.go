package trading_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func (e *routeEnv) routedBuy(t *testing.T) app.ExecuteTrade {
	t.Helper()
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	if err := e.tradeHandler().Handle(actorContext(t.Context()), e.delivery(t, cmd), cmd, nil); err != nil {
		t.Fatal(err)
	}
	return cmd
}

func (e *routeEnv) retry(t *testing.T, cmd app.ExecuteTrade, of uuid.UUID) string {
	t.Helper()
	var inMint, outMint string
	if err := e.pool.QueryRow(t.Context(), `SELECT in_mint, out_mint FROM swaps WHERE id = $1`, of).
		Scan(&inMint, &outMint); err != nil {
		t.Fatal(err)
	}
	ev := busevents.TradeRetryRequested{
		V: 1, SwapID: of, CabalID: e.cabal.UUID(),
		Source: busevents.TradeSource{Kind: "proposal", ID: cmd.ProposalID.UUID()}, Action: "buy", Symbol: cmd.Symbol,
		InMint: chain.SolanaAddress(inMint), OutMint: chain.SolanaAddress(outMint), InAmount: buyMicros,
		QuoteOutAmount: quotedOut, SlippageBps: 100, RequestedBy: e.ids.NewV7(),
	}
	d := e.delivery(t, cmd)
	d.Handler = retryHandler
	if err := (adapters.Engine{Trades: e.tradeHandler()}).HandleRetry(actorContext(t.Context()), d, ev); err != nil {
		t.Fatal(err)
	}
	return e.deliveryCode(t, d)
}

func (e *routeEnv) bridgeOutOf(t *testing.T, swap uuid.UUID) (status, action string) {
	t.Helper()
	if err := e.pool.QueryRow(t.Context(), `SELECT status, coalesce(privy_action_id, '') FROM trade_steps
		WHERE trade_id = $1 AND step = 'bridge_out'`, swap).Scan(&status, &action); err != nil {
		t.Fatal(err)
	}
	return status, action
}

func TestRetry_aRevertedEVMBuySwapsTheStrandedCashWithoutASecondBridge(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	cmd := e.routedBuy(t)
	first := e.swapsOf(t, cmd.ProposalID)[0]
	e.ledger.SetTokens(e.wallet.Address, usdcToken(), 0)
	e.requote()
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}

	if code := e.retry(t, cmd, first); code != string(errs.CodeSwapFailed) {
		t.Fatalf("first retry delivery %s, want swap_failed", code)
	}
	second := e.swapsOf(t, cmd.ProposalID)[1]
	e.requote()
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	if code := e.retry(t, cmd, second); code != "ok" {
		t.Fatalf("second retry delivery %s, want ok", code)
	}

	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 3 || len(e.privy.calls()) != 1 {
		t.Fatalf("%d swaps after %d transfers, want three on the one bridge", len(swaps), len(e.privy.calls()))
	}
	for i, from := range []uuid.UUID{first, second} {
		if status, action := e.bridgeOutOf(t, swaps[i+1]); status != "succeeded" || action != "reused:"+from.String() {
			t.Fatalf("retry %d bridge_out %s %s, want the cash of %s", i+1, status, action, from)
		}
	}
	e.wantEVMRow(t, swaps[2], "0xhash6")
	wantSteps(t, e.steps(t, swaps[2]), "swap:pending:0", "swap:submitted:0", "swap:succeeded:0")
}

func TestRetry_anEVMBuyWhoseBridgeFailedBridgesAgainFromSolanaFunds(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionFailed)
	cmd := e.routedBuy(t)
	first := e.swapsOf(t, cmd.ProposalID)[0]
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	if code := e.retry(t, cmd, first); code != "ok" {
		t.Fatalf("retry delivery %s, want ok", code)
	}
	if n := len(e.privy.calls()); n != 2 {
		t.Fatalf("%d transfers, want a fresh bridge for the retry", n)
	}

	e.privy.script(multichain.ActionFailed)
	again := e.routedBuy(t)
	e.ledger.SetTokens(e.wallet.Address, usdcToken(), 0)
	if code := e.retry(t, again, e.swapsOf(t, again.ProposalID)[0]); code != string(errs.CodeInsufficientFunds) {
		t.Fatalf("retry without stranded cash or Solana funds = %s, want insufficient_funds", code)
	}
}

func TestRetry_anEVMRetryWhoseStrandedCashCannotBeReadIsNotStarted(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	cmd := e.routedBuy(t)
	if _, err := e.pool.Exec(t.Context(), `ALTER TABLE trade_steps RENAME TO trade_steps_gone`); err != nil {
		t.Fatal(err)
	}

	if code := e.retry(t, cmd, e.swapsOf(t, cmd.ProposalID)[0]); code != string(errs.CodeTradeNotStarted) {
		t.Fatalf("retry delivery %s, want trade_not_started", code)
	}
}

func TestRetry_cashAlreadyBridgedBackIsNotReused(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	cmd := e.routedBuy(t)
	first := e.swapsOf(t, cmd.ProposalID)[0]
	if _, err := e.pool.Exec(t.Context(), `INSERT INTO trade_steps
		(id, trade_id, step, network, amount_micros, status, created_at, updated_at)
		VALUES ($1, $2, 'bridge_back', 'base', 24992500, 'pending', now(), now())`, e.ids.NewV7(), first); err != nil {
		t.Fatal(err)
	}
	e.requote()
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	if code := e.retry(t, cmd, first); code != "ok" {
		t.Fatalf("retry delivery %s, want ok", code)
	}
	if n := len(e.privy.calls()); n != 2 {
		t.Fatalf("%d transfers, want a fresh bridge: the stranded cash is on its way back", n)
	}
	if status, action := e.bridgeOutOf(t, e.swapsOf(t, cmd.ProposalID)[1]); status != "succeeded" ||
		action == "reused:"+first.String() {
		t.Fatalf("retry bridge_out %s %s, want its own bridge", status, action)
	}
}

func TestRetry_anEVMBalanceOutageIsRetried(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.balances.FailOnce("TokenBalance", errs.New(errs.CodeRPCUnavailable, "solana.TokenBalance"))
	cmd := e.engineEnv.buy()
	cmd.Route = e.route

	err := e.tradeHandler().Handle(actorContext(t.Context()), e.delivery(t, cmd), cmd, nil)
	if errs.CodeOf(err) != errs.CodeRPCUnavailable || len(e.swapsOf(t, cmd.ProposalID)) != 0 {
		t.Fatalf("Handle err = %v, want rpc_unavailable with no swap", err)
	}
}
