package trading_test

import (
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/jupiter"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

type strandedRig struct {
	*routeEnv
	cmd    app.ExecuteTrade
	first  uuid.UUID
	solana multichain.ListingID
}

func newStrandedRig(t *testing.T) *strandedRig {
	t.Helper()
	e := newRouteEnv(t)
	e.privy.script(multichain.ActionSucceeded)
	e.evm.fills = []app.EVMFill{{State: app.FillReverted}}
	cmd := e.routedBuy(t)
	r := &strandedRig{routeEnv: e, cmd: cmd, first: e.swapsOf(t, cmd.ProposalID)[0]}
	r.solana = e.solanaWinner(t, aaplxToken(), true)
	e.quote(usdcToken(), aaplxToken(), quotedOut, true)
	e.jup.SetOrder(jupiterMint(usdcToken()), jupiterMint(aaplxToken()),
		jupiter.Order{RequestID: "req-solana", Transaction: swapTx()})
	return r
}

func (r *strandedRig) router(evm, solana multichain.ShareNanos) *routerFake {
	winner := r.listing.ID
	if solana > evm {
		winner = r.solana
	}
	return &routerFake{fresh: &multichain.Comparison{
		StockID: r.route.StockID, Side: multichain.SideBuy, Winner: winner,
		Ranked: []multichain.Quote{
			{ListingID: r.listing.ID, Network: chain.NetworkBase, Venue: multichain.VenueUniswap, NetShares: evm},
			{ListingID: r.solana, Network: chain.NetworkSolana, Venue: multichain.VenueJupiter, NetShares: solana},
		},
	}}
}

func (r *strandedRig) handler(router *routerFake) *app.ExecuteTradeHandler {
	return r.reroutingHandler(router, r.withListing(r.winners.listings[r.solana]))
}

func (r *strandedRig) bridgeBacks() int {
	n := 0
	for _, c := range r.privy.calls() {
		if c.key == "trade:"+r.first.String()+":bridge_back" {
			n++
		}
	}
	return n
}

func TestRetry_aStrandedRetryStaysOnItsChainWhileTheEVMRouteHolds(t *testing.T) {
	t.Parallel()
	r := newStrandedRig(t)
	r.requote()
	r.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}
	router := r.router(1_000_000, 900_000)

	code := r.retryThrough(t, r.handler(router), r.cmd, r.first)

	second := r.retrySwap(t, r.cmd.ProposalID)
	status, action := r.bridgeOutOf(t, second)
	if code != "ok" || len(router.asked) != 1 || r.bridgeBacks() != 0 || status != "succeeded" ||
		action != "reused:"+r.first.String() {
		t.Fatalf("retry %s after %v with %d bridge backs, bridge_out %s %s, want the stranded cash swapped in place",
			code, router.asked, r.bridgeBacks(), status, action)
	}
}

func TestRetry_aStrandedRetryBridgesBackAndBuysOnSolanaWhenSolanaWins(t *testing.T) {
	t.Parallel()
	r := newStrandedRig(t)
	r.privy.script(multichain.ActionSucceeded)
	router := r.router(900_000, 1_000_000)

	code := r.retryThrough(t, r.handler(router), r.cmd, r.first)

	second := r.retrySwap(t, r.cmd.ProposalID)
	status, out := r.swapOut(t, second)
	if code != "ok" || r.bridgeBacks() != 1 || status != string(domain.StatusConfirmed) || out != aaplxMint {
		t.Fatalf("retry %s after %d bridge backs: swap %s of %s, want the cash bridged back and bought on Solana",
			code, r.bridgeBacks(), status, out)
	}
	if winner, _, ok := r.routeOf(t, second); !ok || winner != r.solana.UUID() {
		t.Fatalf("retry route %v %s, want the Solana listing", ok, winner)
	}
}

func TestRetry_aCrashBetweenTheBridgeBackAndTheSolanaBuyBuysOnceWithoutASecondBridge(t *testing.T) {
	t.Parallel()
	r := newStrandedRig(t)
	r.privy.script(multichain.ActionSucceeded)
	r.balances.FailOnce("TokenBalance", errs.New(errs.CodeRPCUnavailable, "solana.TokenBalance"))
	h := r.handler(r.router(900_000, 1_000_000))

	if _, err := r.retryHandle(t, h, r.cmd, r.first); err == nil || r.bridgeBacks() != 1 {
		t.Fatalf("first delivery err = %v after %d bridge backs, want the Solana buy to fail after the bridge back",
			err, r.bridgeBacks())
	}
	code := r.retryThrough(t, h, r.cmd, r.first)

	swaps := r.swapsOf(t, r.cmd.ProposalID)
	status, out := r.swapOut(t, swaps[len(swaps)-1])
	if code != "ok" || r.bridgeBacks() != 1 || status != string(domain.StatusConfirmed) || out != aaplxMint {
		t.Fatalf("redelivery %s after %d bridge backs: swap %s of %s, want one Solana buy on the returned cash",
			code, r.bridgeBacks(), status, out)
	}
}

func TestRetry_aBridgeBackStillInFlightIsRetried(t *testing.T) {
	t.Parallel()
	r := newStrandedRig(t)
	h := r.handler(r.router(900_000, 1_000_000))

	if _, err := r.retryHandle(t, h, r.cmd, r.first); !errs.Retryable(errs.CodeOf(err)) ||
		len(r.swapsOf(t, r.cmd.ProposalID)) != 1 {
		t.Fatalf("delivery err = %v with %d swaps, want a retryable wait and no Solana buy yet", err,
			len(r.swapsOf(t, r.cmd.ProposalID)))
	}
	r.privy.script(multichain.ActionSucceeded)
	if code := r.retryThrough(t, h, r.cmd, r.first); code != "ok" || r.bridgeBacks() != 1 {
		t.Fatalf("redelivery %s after %d bridge backs, want the landed cash bought once", code, r.bridgeBacks())
	}
}

func TestRetry_aStrandedRetryStaysWithoutAUsableSolanaQuote(t *testing.T) {
	t.Parallel()
	for name, router := range map[string]func(r *strandedRig) *routerFake{
		"router failed": func(*strandedRig) *routerFake {
			return &routerFake{err: errs.New(errs.CodeNoRoute, "market.Router.Best")}
		},
		"no Solana quote": func(r *strandedRig) *routerFake {
			f := r.router(900_000, 1_000_000)
			f.fresh.Ranked = f.fresh.Ranked[:1]
			return f
		},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newStrandedRig(t)
			r.requote()
			r.evm.fills = []app.EVMFill{{State: app.FillSucceeded, Out: filledOut}}

			code := r.retryThrough(t, r.handler(router(r)), r.cmd, r.first)

			_, action := r.bridgeOutOf(t, r.retrySwap(t, r.cmd.ProposalID))
			if code != "ok" || r.bridgeBacks() != 0 || action != "reused:"+r.first.String() {
				t.Fatalf("retry %s after %d bridge backs, bridge_out %s, want the cash swapped in place",
					code, r.bridgeBacks(), action)
			}
		})
	}
}

func TestRetry_aStrandedRetryStopsWhenTheCashCannotComeBack(t *testing.T) {
	t.Parallel()
	for name, c := range map[string]struct {
		arrange func(r *strandedRig)
		want    errs.Code
	}{
		"bridge back failed": {func(r *strandedRig) { r.privy.script(multichain.ActionFailed) }, errs.CodeSwapFailed},
		"bridge back unsent": {func(r *strandedRig) {
			r.privy.transferErr = errs.New(errs.CodePrivyUnavailable, "privy.Transfer")
		}, errs.CodePrivyUnavailable},
		"catalog down after the landing": {func(r *strandedRig) {
			r.privy.script(multichain.ActionSucceeded)
			r.catalog.Fail("AssetBySymbol", errs.New(errs.CodeRPCUnavailable, "market.AssetBySymbol"))
		}, errs.CodeRPCUnavailable},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			r := newStrandedRig(t)
			c.arrange(r)
			d, err := r.retryHandle(t, r.handler(r.router(900_000, 1_000_000)), r.cmd, r.first)
			if errs.CodeOf(err) != c.want && r.deliveryCode(t, d) != string(errs.CodeTradeNotStarted) ||
				len(r.swapsOf(t, r.cmd.ProposalID)) != 1 {
				t.Fatalf("delivery err = %v with %d swaps, want %s and no Solana buy", err,
					len(r.swapsOf(t, r.cmd.ProposalID)), c.want)
			}
		})
	}
}

func TestRetry_aStrandedRetryWaitsOutARouterOutage(t *testing.T) {
	t.Parallel()
	r := newStrandedRig(t)
	router := &routerFake{err: errs.New(errs.CodeUpstreamUnavailable, "market.Router.Best")}

	_, err := r.retryHandle(t, r.handler(router), r.cmd, r.first)

	if !errs.Retryable(errs.CodeOf(err)) || r.bridgeBacks() != 0 || len(r.swapsOf(t, r.cmd.ProposalID)) != 1 {
		t.Fatalf(
			"delivery err = %v after %d bridge backs, want a retryable wait and nothing moved",
			err,
			r.bridgeBacks(),
		)
	}
}
