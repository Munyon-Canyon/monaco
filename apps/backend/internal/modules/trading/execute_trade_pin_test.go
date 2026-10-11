package trading_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	busevents "github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/cabal"
	governance "github.com/monaco/monaco/apps/backend/internal/modules/governance/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type networkRouter struct {
	routerFake
	on    *multichain.Comparison
	onErr error
	asked []chain.Network
}

func (r *networkRouter) BestOn(
	_ context.Context, _ multichain.StockID, _ multichain.Side, _ money.Micros, n chain.Network,
) (multichain.Comparison, error) {
	r.asked = append(r.asked, n)
	if r.onErr != nil {
		return multichain.Comparison{}, r.onErr
	}
	return *r.on, nil
}

func (e *routeEnv) solanaFirst(t *testing.T) (*multichain.Comparison, multichain.ListingID) {
	t.Helper()
	solana := e.solanaWinner(t, aaplxToken(), true)
	return &multichain.Comparison{
		StockID: e.route.StockID, Side: multichain.SideBuy, Winner: solana,
		Ranked: []multichain.Quote{
			{
				ListingID: solana, Network: chain.NetworkSolana, Venue: multichain.VenueJupiter,
				NetShares: 1_000_000, InputMicros: money.MicrosFromUint64(buyMicros),
			},
			{
				ListingID: e.listing.ID, Network: chain.NetworkBase, Venue: multichain.VenueUniswap,
				NetShares: 900_000, InputMicros: money.MicrosFromUint64(buyMicros),
				Costs: []multichain.Cost{
					{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(bridgeFeeMicros)},
				},
			},
		},
	}, solana
}

func (e *routeEnv) evmFills() {
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	e.quote(usdcToken(), aaplxToken(), 0, false)
}

func (e *routeEnv) passedOnBase(t *testing.T, h *app.ExecuteTradeHandler, cmd app.ExecuteTrade) string {
	t.Helper()
	d := e.delivery(t, cmd)
	ev := busevents.ProposalPassed{
		V: 1, ProposalID: cmd.ProposalID.UUID(), CabalID: cmd.CabalID.UUID(), Kind: string(cmd.Action),
		Symbol: cmd.Symbol, Mint: cmd.Mint, USDCMicros: cmd.USDCMicros, QuoteOutAmount: cmd.QuoteOutAmount,
		ProposerID: e.ids.NewV7(), PriceComparison: cmd.Route, Network: chain.NetworkBase,
	}
	if err := (adapters.Engine{Trades: h}).Handle(actorContext(t.Context()), d, ev); err != nil {
		t.Fatal(err)
	}
	return e.deliveryCode(t, d)
}

func (e *routeEnv) pinnedRoute(t *testing.T, swap uuid.UUID) (uuid.UUID, string) {
	t.Helper()
	var winner uuid.UUID
	var pin string
	if err := e.pool.QueryRow(t.Context(),
		`SELECT winner_listing_id, coalesce(pinned_network, '') FROM trade_routes WHERE trade_id = $1`, swap).
		Scan(&winner, &pin); err != nil {
		t.Fatal(err)
	}
	return winner, pin
}

func (e *routeEnv) wantPinRefused(t *testing.T, cmd app.ExecuteTrade, code string, want errs.Code) {
	t.Helper()
	blocked := e.blocked(t, cmd.ProposalID)
	swaps, steps := len(e.swapsOf(t, cmd.ProposalID)), e.count(t, `SELECT count(*) FROM trade_steps`)
	if code != string(want) || len(blocked) != 1 || blocked[0]["code"] != string(want) || swaps != 0 || steps != 0 {
		t.Fatalf("delivery %s, blocked %v, %d swaps, %d steps, want %s with no swap and no step",
			code, blocked, swaps, steps, want)
	}
}

func TestExecuteTrade_aBaseBuyPinnedOverASolanaWinnerRunsTheRoutedEVMBuy(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.seedCabal(cabal.StatusActive, 250)
	e.evmFills()
	fresh, solana := e.solanaFirst(t)
	cmd := e.engineEnv.buy()
	cmd.Route = e.route

	h := e.reroutingHandler(&routerFake{fresh: fresh}, e.withListing(e.winners.listings[solana]))

	code := e.passedOnBase(t, h, cmd)

	swaps := e.swapsOf(t, cmd.ProposalID)
	if code != "ok" || len(swaps) != 1 || len(e.privy.calls()) != 1 {
		t.Fatalf("delivery %s, %d swaps, %d bridges, want one routed EVM buy", code, len(swaps), len(e.privy.calls()))
	}
	e.wantEVMRow(t, swaps[0], swapHash)
	if winner, pin := e.pinnedRoute(t, swaps[0]); winner != e.listing.ID.UUID() || pin != "base" {
		t.Fatalf("route %s pinned %q, want the Base listing pinned to base", winner, pin)
	}
	if bps := e.count(t, `SELECT slippage_bps FROM swaps WHERE id = $1`, swaps[0]); bps != 250 {
		t.Fatalf("slippage_bps = %d, want the cabal's 250", bps)
	}
}

func TestExecuteTrade_aBasePinWithNoBaseRouteIsBlocked(t *testing.T) {
	t.Parallel()
	for name, router := range map[string]func(t *testing.T, e *routeEnv) multichain.Router{
		"no base quote": func(t *testing.T, e *routeEnv) multichain.Router {
			t.Helper()
			fresh, _ := e.solanaFirst(t)
			fresh.Ranked = fresh.Ranked[:1]
			return &routerFake{fresh: fresh}
		},
		"router refused": func(*testing.T, *routeEnv) multichain.Router {
			return &routerFake{err: errs.New(errs.CodeNoRoute, "market.Router.Best")}
		},
		"no router and no base quote in the vote": func(*testing.T, *routeEnv) multichain.Router { return nil },
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			e := newRouteEnv(t)
			cmd := e.engineEnv.buy()
			cmd.Route = e.route
			r := router(t, e)
			if r == nil {
				voted, _ := e.solanaFirst(t)
				voted.Ranked = voted.Ranked[:1]
				cmd.Route = voted
			}

			code := e.passedOnBase(t, e.reroutingHandler(r, e.listings), cmd)

			e.wantPinRefused(t, cmd, code, errs.CodePinnedChainUnavailable)
		})
	}
}

func TestExecuteTrade_aSolanaPinKeepsTheSolanaFallbackWhenTheRouterRefuses(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	ondo := e.ondoOrder(t)
	cmd := e.buy()
	cmd.Route = e.comparison(t, e.solanaWinner(t, ondo, true))
	cmd.Pin = chain.NetworkSolana
	router := &routerFake{err: errs.New(errs.CodeNoRoute, "market.Router.Best")}
	d := e.delivery(t, cmd)

	if err := e.routedHandler(router).Handle(actorContext(t.Context()), d, cmd, nil); err != nil {
		t.Fatal(err)
	}
	if code, out := e.deliveryCode(t, d), e.outMint(t, cmd.ProposalID); code != "ok" || out != aaplxMint {
		t.Fatalf("delivery %s bought %s, want the proposal mint on Solana", code, out)
	}
}

func TestExecuteTrade_aBasePinWithEVMTradingOffIsBlocked(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	h := app.NewExecuteTradeHandler(app.ExecuteTradeDeps{
		Layer: e.layer(), Routes: e.exec(), Listings: e.listings, UoW: e.uow, Reads: e.reads, Venue: e.venue,
		Ports: e.ports(), USDC: usdcToken(),
	})

	code := e.passedOnBase(t, h, cmd)

	e.wantPinRefused(t, cmd, code, errs.CodeChainTradingOff)
}

func TestExecuteTrade_aBasePinStillBuysOnBaseWhenSolanaCannotBeQuoted(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	e.evmFills()
	fresh, _ := e.solanaFirst(t)
	on := *fresh
	on.Ranked, on.Winner = fresh.Ranked[1:], e.listing.ID
	unquoted := errs.New(errs.CodeQuoteUnavailable, "market.Router.Best")
	router := &networkRouter{routerFake: routerFake{err: unquoted}, on: &on}
	cmd := e.engineEnv.buy()
	cmd.Route = e.route

	code := e.passedOnBase(t, e.reroutingHandler(router, e.listings), cmd)

	swaps := e.swapsOf(t, cmd.ProposalID)
	if code != "ok" || len(swaps) != 1 || len(router.asked) != 1 || router.asked[0] != chain.NetworkBase ||
		len(router.routerFake.asked) != 0 {
		t.Fatalf("delivery %s, %d swaps, asked %v on and %d best, want one Base-only route", code, len(swaps),
			router.asked, len(router.routerFake.asked))
	}
	e.wantEVMRow(t, swaps[0], swapHash)
}

func TestExecuteTrade_aBasePinWhoseBaseQuoteFailsIsBlocked(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	router := &networkRouter{onErr: errs.New(errs.CodeNoRoute, "market.Router.Best")}
	cmd := e.engineEnv.buy()
	cmd.Route = e.route

	code := e.passedOnBase(t, e.reroutingHandler(router, e.listings), cmd)

	e.wantPinRefused(t, cmd, code, errs.CodePinnedChainUnavailable)
}

func (e *routeEnv) failedPinnedBuy(t *testing.T) (app.ExecuteTrade, uuid.UUID) {
	t.Helper()
	e.privy.script(multichain.ActionFailed)
	cmd := e.engineEnv.buy()
	cmd.Route = e.route
	e.passedOnBase(t, e.tradeHandler(), cmd)
	first := e.swapsOf(t, cmd.ProposalID)[0]
	if _, pin := e.pinnedRoute(t, first); pin != "base" {
		t.Fatalf("first route pinned %q, want base", pin)
	}
	return cmd, first
}

func TestRetry_aBasePinnedRetryStaysOnBaseWhenSolanaWinsTheFreshComparison(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd, first := e.failedPinnedBuy(t)
	e.privy.script(multichain.ActionSucceeded)
	e.requote()
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	fresh, solana := e.solanaFirst(t)

	code := e.retryThrough(t, e.reroutingHandler(&routerFake{fresh: fresh}, e.withListing(e.winners.listings[solana])),
		cmd, first)

	second := e.retrySwap(t, cmd.ProposalID)
	if winner, pin := e.pinnedRoute(t, second); code != "ok" || winner != e.listing.ID.UUID() || pin != "base" {
		t.Fatalf("retry %s routed %s pinned %q, want the Base listing pinned to base", code, winner, pin)
	}
}

func (e *routeEnv) pin(t *testing.T, swap uuid.UUID) {
	t.Helper()
	if _, err := e.pool.Exec(t.Context(), `UPDATE trade_routes SET pinned_network = 'base' WHERE trade_id = $1`,
		swap); err != nil {
		t.Fatal(err)
	}
}

func TestRetry_aBasePinnedStrandedRetryNeverBridgesBackToSolana(t *testing.T) {
	t.Parallel()
	r := newStrandedRig(t)
	r.pin(t, r.first)
	r.requote()
	r.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	router := r.router(900_000, 1_000_000)

	code := r.retryThrough(t, r.handler(router), r.cmd, r.first)

	second := r.retrySwap(t, r.cmd.ProposalID)
	winner, pin := r.pinnedRoute(t, second)
	if code != "ok" || r.bridgeBacks() != 0 || len(router.asked) != 0 || winner != r.listing.ID.UUID() ||
		pin != "base" {
		t.Fatalf("retry %s after %d bridge backs and %v, route %s pinned %q, want the stranded cash bought on Base",
			code, r.bridgeBacks(), router.asked, winner, pin)
	}
}

func TestRetry_aBasePinnedRetryOfReturnedCashRoutesOnBaseAgain(t *testing.T) {
	t.Parallel()
	e := newRouteEnv(t)
	cmd, swap := e.strand(t)
	e.pin(t, swap)
	e.mustTick(t, 1, 0)
	e.proposals.set(governance.Status("cancelled"))
	e.privy.script(multichain.ActionSucceeded)
	e.mustTick(t, 1, 1)
	e.proposals.set(governance.StatusPassed)
	e.privy.script(multichain.ActionSucceeded)
	e.requote()
	e.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	fresh, solana := e.solanaFirst(t)

	code := e.retryThrough(t, e.reroutingHandler(&routerFake{fresh: fresh}, e.withListing(e.winners.listings[solana])),
		cmd, swap)

	second := e.retrySwap(t, cmd.ProposalID)
	if winner, pin := e.pinnedRoute(t, second); code != "ok" || winner != e.listing.ID.UUID() || pin != "base" {
		t.Fatalf("retry %s routed %s pinned %q, want the returned cash routed on Base", code, winner, pin)
	}
}

func TestLazyRouter_routesOnOneNetworkThroughTheInnerRouter(t *testing.T) {
	t.Parallel()
	want := &multichain.Comparison{Winner: multichain.ListingIDFrom(testkit.NewIDs(5263).NewV7())}
	inner := &networkRouter{on: want}
	r := adapters.NewLazyRouter(func() multichain.Router { return inner })

	got, err := r.BestOn(t.Context(), multichain.StockID{}, multichain.SideBuy, money.Micros{}, chain.NetworkBase)

	if err != nil || got.Winner != want.Winner || len(inner.asked) != 1 || inner.asked[0] != chain.NetworkBase {
		t.Fatalf("BestOn = %s, %v after %v, want the inner router's Base comparison", got.Winner, err, inner.asked)
	}
}
