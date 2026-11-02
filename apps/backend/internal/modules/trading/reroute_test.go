package trading_test

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type routerFake struct {
	fresh *multichain.Comparison
	err   error
	asked []multichain.StockID
}

func (r *routerFake) Best(
	_ context.Context, stock multichain.StockID, side multichain.Side, _ money.Micros,
) (multichain.Comparison, error) {
	r.asked = append(r.asked, stock)
	if r.err != nil || side != multichain.SideBuy {
		return multichain.Comparison{}, r.err
	}
	return *r.fresh, nil
}

func (e *engineEnv) routedHandler(router multichain.Router) *app.ExecuteTradeHandler {
	return app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), Listings: e.winners, UoW: e.uow, Reads: e.reads, Venue: e.venue, Ports: e.ports(),
		USDC: usdcToken(), Router: router,
	})
}

func (e *engineEnv) outMint(t *testing.T, proposal ids.ProposalID) string {
	t.Helper()
	var out string
	if err := e.pool.QueryRow(t.Context(), `SELECT out_mint FROM swaps WHERE source_id = $1`, proposal.UUID()).
		Scan(&out); err != nil {
		t.Fatal(err)
	}
	return out
}

func (e *engineEnv) ondoOrder(t *testing.T) chain.Mint {
	t.Helper()
	ondo := chain.Mint{Address: "AAPLonDo1111111111111111111111111111111111111", Decimals: 9}
	e.quote(usdcToken(), ondo, 210_000_000, true)
	order := jupiter.Order{RequestID: "req-1", Transaction: swapTx()}
	e.jup.SetOrder(jupiterMint(usdcToken()), jupiterMint(ondo), order)
	return ondo
}

func TestExecuteTrade_buysTheExecutionTimeWinnerAndRecordsItsComparison(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	ondo := e.ondoOrder(t)
	voted := e.comparison(t, e.solanaWinner(t, aaplxToken(), true))
	freshWinner := e.solanaWinner(t, ondo, true)
	fresh := e.comparison(t, freshWinner)
	fresh.Ranked[0].RouteID = "route-fresh"
	router := &routerFake{fresh: fresh}
	cmd := e.buy()
	cmd.Route = voted

	if err := e.routedHandler(router).Handle(actorContext(t.Context()), e.delivery(t, cmd), cmd, nil); err != nil {
		t.Fatal(err)
	}
	if out := e.outMint(t, cmd.ProposalID); out != string(ondo.Address) {
		t.Fatalf("bought %s, want the execution-time winner %s", out, ondo.Address)
	}
	winner, route, ok := e.routeOf(t, e.swapsOf(t, cmd.ProposalID)[0])
	if !ok || winner != freshWinner.UUID() || route != "route-fresh" {
		t.Fatalf("recorded %v %s %q, want the fresh comparison", ok, winner, route)
	}
	if len(router.asked) != 1 || router.asked[0] != voted.StockID {
		t.Fatalf("router asked %v, want the voted stock once", router.asked)
	}
}

func TestExecuteTrade_aRouterFailureBuysTheProposalMintOnSolana(t *testing.T) {
	t.Parallel()
	for name, code := range map[string]errs.Code{"no route": errs.CodeNoRoute, "decode": errs.CodeDecodeFailed} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEngineEnv(t)
			ondo := e.ondoOrder(t)
			cmd := e.buy()
			cmd.Route = e.comparison(t, e.solanaWinner(t, ondo, true))
			router := &routerFake{err: errs.New(code, "market.Router.Best")}

			h := e.routedHandler(router)
			if err := h.Handle(actorContext(t.Context()), e.delivery(t, cmd), cmd, nil); err != nil {
				t.Fatal(err)
			}
			swaps := e.swapsOf(t, cmd.ProposalID)
			if out := e.outMint(t, cmd.ProposalID); out != aaplxMint {
				t.Fatalf("bought %s, want the proposal's own mint", out)
			}
			if _, _, ok := e.routeOf(t, swaps[0]); ok {
				t.Fatal("a route was recorded for a trade the router could not route")
			}
		})
	}
}

func TestExecuteTrade_aRetryableRouterErrorRetriesWithoutASwap(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()
	cmd.Route = e.comparison(t, e.solanaWinner(t, aaplxToken(), true))
	router := &routerFake{err: errs.New(errs.CodeUpstreamUnavailable, "market.Router.Best")}

	err := e.routedHandler(router).Handle(actorContext(t.Context()), e.delivery(t, cmd), cmd, nil)

	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || len(e.swapsOf(t, cmd.ProposalID)) != 0 {
		t.Fatalf("Handle err = %v with %d swaps, want upstream_unavailable and none", err,
			len(e.swapsOf(t, cmd.ProposalID)))
	}
}

func (e *routeEnv) reroutingHandler(router multichain.Router, listings listingsFake) *app.ExecuteTradeHandler {
	e.listings = listings
	return app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), Routes: e.exec(), Listings: listings, UoW: e.uow, Reads: e.reads, Venue: e.venue,
		Ports: e.ports(), USDC: usdcToken(), EVM: true, Router: router,
	})
}

func (e *routeEnv) retryThrough(t *testing.T, h *app.ExecuteTradeHandler, cmd app.ExecuteTrade, of uuid.UUID) string {
	t.Helper()
	d, err := e.retryHandle(t, h, cmd, of)
	if err != nil {
		t.Fatal(err)
	}
	return e.deliveryCode(t, d)
}

func (e *routeEnv) retryHandle(
	t *testing.T, h *app.ExecuteTradeHandler, cmd app.ExecuteTrade, of uuid.UUID,
) (bus.Delivery, error) {
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
	return d, (adapters.Engine{Trades: h}).HandleRetry(actorContext(t.Context()), d, ev)
}

func (e *routeEnv) retrySwap(t *testing.T, proposal ids.ProposalID) uuid.UUID {
	t.Helper()
	swaps := e.swapsOf(t, proposal)
	if len(swaps) != 2 {
		t.Fatalf("%d swaps, want the first and its retry", len(swaps))
	}
	return swaps[1]
}

func (e *routeEnv) swapOut(t *testing.T, id uuid.UUID) (string, string) {
	t.Helper()
	var status, out string
	if err := e.pool.QueryRow(t.Context(), `SELECT status, out_mint FROM swaps WHERE id = $1`, id).
		Scan(&status, &out); err != nil {
		t.Fatal(err)
	}
	return status, out
}

func (e *routeEnv) withListing(l multichain.Listing) listingsFake {
	all := map[multichain.ListingID]multichain.Listing{l.ID: l}
	for id, existing := range e.listings.listings {
		all[id] = existing
	}
	return listingsFake{listings: all}
}

func (e *routeEnv) failedEVMBuy(t *testing.T) (app.ExecuteTrade, uuid.UUID) {
	t.Helper()
	e.privy.script(multichain.ActionFailed)
	cmd := e.routedBuy(t)
	first := e.swapsOf(t, cmd.ProposalID)[0]
	if _, out := e.swapOut(t, first); !strings.HasPrefix(out, "base:") {
		t.Fatalf("failed buy out_mint %s, want the EVM asset", out)
	}
	return cmd, first
}

func (e *routeEnv) ethereumListing(t *testing.T) multichain.Listing {
	t.Helper()
	id := multichain.ListingIDFrom(e.ids.NewV7())
	token := "0x00000000000000000000000000000000000e7e01"
	if _, err := e.pool.Exec(t.Context(), `INSERT INTO asset_listings
		(id, issuer, symbol, network, chain_id, token_address, decimals, shares_per_token, venue, enabled, created_at)
		VALUES ($1, 'ondo', 'AAPLon', 'ethereum', 1, $2, 18, 1, 'uniswap', false, $3)`,
		id.UUID(), token, e.clk.Now()); err != nil {
		t.Fatal(err)
	}
	return multichain.Listing{
		ID: id, Symbol: "AAPLon", Network: chain.NetworkEthereum, TokenAddress: token, Decimals: 18,
		Venue: multichain.VenueUniswap,
	}
}

func TestRetry_anEVMBuyRetryBuysAFreshSolanaWinner(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd, first := e.failedEVMBuy(t)
	ondo := e.ondoOrder(t)
	winner := e.solanaWinner(t, ondo, true)
	router := &routerFake{fresh: e.comparison(t, winner)}

	code := e.retryThrough(t, e.reroutingHandler(router, e.withListing(e.winners.listings[winner])), cmd, first)

	second := e.retrySwap(t, cmd.ProposalID)
	status, out := e.swapOut(t, second)
	if code != "ok" || status != string(domain.StatusConfirmed) || out != string(ondo.Address) {
		t.Fatalf("retry %s: swap %s of %s, want the fresh Solana winner bought", code, status, out)
	}
	if got, _, ok := e.routeOf(t, second); !ok || got != winner.UUID() || len(router.asked) != 1 {
		t.Fatalf("route %v %s after %v, want the fresh comparison", ok, got, router.asked)
	}
}

func TestRetry_anEVMBuyRetryBuysAFreshWinnerOnAnotherEVMNetwork(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd, first := e.failedEVMBuy(t)
	eth := e.ethereumListing(t)
	fresh := e.comparison(t, eth.ID)
	fresh.Ranked[0].Network = chain.NetworkEthereum
	router := &routerFake{fresh: fresh}
	e.privy.script(multichain.ActionSucceeded)
	e.requote()
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

	code := e.retryThrough(t, e.reroutingHandler(router, e.withListing(eth)), cmd, first)

	second := e.retrySwap(t, cmd.ProposalID)
	status, out := e.swapOut(t, second)
	if code != "ok" || status != string(domain.StatusConfirmed) || out != "ethereum:"+eth.TokenAddress {
		t.Fatalf("retry %s: swap %s of %s, want the Ethereum winner bought", code, status, out)
	}
}

func TestRetry_aRouterFailureOnAnEVMBuyRetryBuysTheResolvedSolanaMint(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd, first := e.failedEVMBuy(t)
	e.quote(usdcToken(), aaplxToken(), quotedOut, true)
	router := &routerFake{err: errs.New(errs.CodeNoRoute, "market.Router.Best")}

	code := e.retryThrough(t, e.reroutingHandler(router, e.listings), cmd, first)

	second := e.retrySwap(t, cmd.ProposalID)
	status, out := e.swapOut(t, second)
	if code != "ok" || status != string(domain.StatusConfirmed) || out != aaplxMint {
		t.Fatalf("retry %s: swap %s of %s, want the catalog's Solana mint bought through Jupiter", code, status, out)
	}
}

type countingRouter struct{ calls int }

func (c *countingRouter) Best(
	context.Context, multichain.StockID, multichain.Side, money.Micros,
) (multichain.Comparison, error) {
	c.calls++
	return multichain.Comparison{}, errs.New(errs.CodeNoRoute, "test.Best")
}

func TestLazyRouter_buildsTheRouterOnceOnFirstUse(t *testing.T) {
	t.Parallel()
	built, inner := 0, &countingRouter{}
	r := adapters.NewLazyRouter(func() multichain.Router {
		built++
		return inner
	})
	for range 2 {
		if _, err := r.Best(t.Context(), multichain.StockID{}, multichain.SideBuy, money.Micros{}); errs.CodeOf(err) !=
			errs.CodeNoRoute {
			t.Fatalf("Best err = %v, want the inner router's no_route", err)
		}
	}
	if built != 1 || inner.calls != 2 {
		t.Fatalf("built %d times, %d calls, want one build and both calls through it", built, inner.calls)
	}
}

func TestModule_routesABuyThroughMarketsRouterAndFallsBackToSolanaForAnUnknownStock(t *testing.T) {
	t.Parallel()
	cfg := testkit.Config()
	cfg.Privy.BaseURL, cfg.Timeouts.Privy, cfg.Routing.UniswapTimeout = "http://privy.invalid", time.Second, time.Second
	cfg.Jupiter.SwapBaseURL, cfg.Jupiter.PriceBaseURL = "http://jup.invalid", "http://jup.invalid"
	cfg.Jupiter.RateLimit = 8
	cfg.Timeouts.JupiterQuote, cfg.Timeouts.JupiterExecute = time.Second, time.Second
	b := newEngineBusWith(t, cfg)
	ondo := b.ondoOrder(t)
	b.quote(usdcToken(), aaplxToken(), quotedOut, true)
	cmd := b.buy()
	cmd.Route = b.comparison(t, b.solanaWinner(t, ondo, true))

	b.dispatch(t, b.message(t, cmd, ""))

	if out := b.outMint(t, cmd.ProposalID); out != aaplxMint {
		t.Fatalf("bought %s, want the proposal mint after market's router found no route", out)
	}
}

func TestExecuteTrade_aRetryWhoseStoredRouteCannotBeReadIsNotStarted(t *testing.T) {
	t.Parallel()
	for name, breakIt := range map[string]string{
		"unreadable":  `ALTER TABLE trade_routes RENAME TO trade_routes_gone`,
		"undecodable": `UPDATE trade_routes SET comparison = '"not a comparison"'`,
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newEngineEnv(t)
			cmd := e.buy()
			cmd.Route = e.comparison(t, e.solanaWinner(t, aaplxToken(), true))
			e.jup.FailOnce("Order", errs.New(errs.CodeJupiterRejected, "jupiter.Order"))
			if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
				t.Fatal(err)
			}
			if _, err := e.pool.Exec(t.Context(), breakIt); err != nil {
				t.Fatal(err)
			}
			retry := e.buy()
			retry.ProposalID = cmd.ProposalID
			retry.Retry = &app.Retry{Of: ids.SwapIDFrom(e.swapsOf(t, cmd.ProposalID)[0])}
			d := e.delivery(t, retry)
			d.Final = true

			if err := e.routedHandler(&routerFake{}).Handle(actorContext(t.Context()), d, retry, nil); err != nil {
				t.Fatal(err)
			}
			if code := e.deliveryCode(t, d); code != string(errs.CodeTradeNotStarted) {
				t.Fatalf("delivery %s, want trade_not_started", code)
			}
		})
	}
}

func TestRetry_anEVMBuyRetryWhoseSymbolCannotBeResolved(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		code errs.Code
		want string
	}{
		"not in the catalog": {errs.CodeAssetNotFound, string(errs.CodeAssetUntradable)},
		"catalog outage":     {errs.CodeRPCUnavailable, ""},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			cmd, first := e.failedEVMBuy(t)
			e.catalog.Fail("AssetBySymbol", errs.New(c.code, "market.AssetBySymbol"))
			router := &routerFake{err: errs.New(errs.CodeNoRoute, "market.Router.Best")}

			d, err := e.retryHandle(t, e.reroutingHandler(router, e.listings), cmd, first)

			if c.want == "" {
				if errs.CodeOf(err) != c.code || len(router.asked) != 0 {
					t.Fatalf("Handle err = %v after %v, want %s before routing", err, router.asked, c.code)
				}
				return
			}
			if err != nil || e.deliveryCode(t, d) != c.want {
				t.Fatalf("Handle err = %v, want the trade refused as %s", err, c.want)
			}
		})
	}
}
