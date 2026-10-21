package trading_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (e *engineEnv) swapOutcome(t *testing.T, proposal ids.ProposalID) (string, string) {
	t.Helper()
	var status, outMint string
	if err := e.pool.QueryRow(t.Context(), `SELECT status, out_mint FROM swaps WHERE source_id = $1`,
		proposal.UUID()).Scan(&status, &outMint); err != nil {
		t.Fatal(err)
	}
	return status, outMint
}

func TestExecuteTrade_aSolanaWinnerFromTheMarketCatalogBuysThroughJupiter(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	ondo := chain.Mint{Address: "88a8Dstsn9NgnpiThVQEgRaWKNr4jFxjKuKbut21N2Da", Decimals: 9}
	winner := e.solanaWinner(t, ondo, true)
	if _, err := e.pool.Exec(t.Context(), `UPDATE asset_listings SET enabled = true, verified_at = $2 WHERE id = $1`,
		winner.UUID(), e.clk.Now()); err != nil {
		t.Fatal(err)
	}
	e.quote(usdcToken(), ondo, 210_000_000, true)
	order := jupiter.Order{RequestID: "req-1", Transaction: swapTx()}
	e.jup.SetOrder(jupiterMint(usdcToken()), jupiterMint(ondo), order)
	cmd := e.buy()
	cmd.Route = e.comparison(t, winner)
	h := app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), Listings: market.New(module.Deps{Pool: e.pool}).Catalog(), UoW: e.uow, Reads: e.reads,
		Venue: e.venue, Ports: e.ports(), USDC: usdcToken(),
	})

	if err := h.Handle(actorContext(t.Context()), e.delivery(t, cmd), cmd, nil); err != nil {
		t.Fatal(err)
	}
	status, outMint := e.swapOutcome(t, cmd.ProposalID)
	if status != "confirmed" || outMint != string(ondo.Address) {
		t.Fatalf("swap %s of %s, want the catalog's Solana winner confirmed", status, outMint)
	}
	if got, _, ok := e.routeOf(t, e.swapsOf(t, cmd.ProposalID)[0]); !ok || got != winner.UUID() {
		t.Fatalf("route %v %s, want the comparison recorded with winner %s", ok, got, winner.UUID())
	}
}

func TestExecuteTrade_anEVMWinnerWithEVMOffBuysTheProposalMintOnSolana(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.quote(usdcToken(), aaplxToken(), quotedOut, true)
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	h := app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), Routes: e.exec(), Listings: e.listings, UoW: e.uow, Reads: e.reads, Venue: e.venue,
		Ports: e.ports(), USDC: usdcToken(),
	})

	if err := h.Handle(actorContext(t.Context()), e.delivery(t, cmd), cmd, nil); err != nil {
		t.Fatal(err)
	}
	status, outMint := e.swapOutcome(t, cmd.ProposalID)
	if status != "confirmed" || outMint != aaplxMint {
		t.Fatalf("swap %s of %s, want the proposal's own mint confirmed through Jupiter", status, outMint)
	}
	steps := e.count(t, `SELECT count(*) FROM trade_steps WHERE trade_id = $1`, e.swapsOf(t, cmd.ProposalID)[0])
	if steps != 0 || len(e.privy.calls()) != 0 {
		t.Fatalf("%d trade steps and %d transfers, want no route and no bridge", steps, len(e.privy.calls()))
	}
}

func TestModule_routesAnEVMWinnerOnlyWithEVMOn(t *testing.T) {
	t.Parallel()
	for name, evm := range map[string]bool{"off": false, "on": true} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			route := newRouteEnv(t)
			ports := app.RoutePorts{Listings: route.listings, Router: staticRouter{route.route}, EVM: evm}
			route.options = []trading.Option{trading.WithRoutePorts(ports)}
			b := newEngineBusOn(t, testkit.Config(), route.engineEnv)
			b.quote(usdcToken(), aaplxToken(), quotedOut, true)
			cmd := b.buy()
			cmd.Route = route.route

			b.dispatch(t, b.message(t, cmd, ""))

			solana := b.count(t, `SELECT count(*) FROM swaps WHERE source_id = $1 AND out_mint = $2
				AND status = 'confirmed'`, cmd.ProposalID.UUID(), aaplxMint)
			if want := map[bool]int{false: 1, true: 0}[evm]; solana != want {
				t.Fatalf("%d confirmed Solana swaps of the proposal mint, want %d", solana, want)
			}
		})
	}
}

type staticRouter struct{ c *multichain.Comparison }

func (r staticRouter) Best(
	context.Context, multichain.StockID, multichain.Side, money.Micros,
) (multichain.Comparison, error) {
	return *r.c, nil
}
