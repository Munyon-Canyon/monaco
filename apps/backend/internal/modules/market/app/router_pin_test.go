package app_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestRouterRoutesOneNetworkWithoutTheSolanaQuote(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(5263)
	byName := appleListings(t, g)
	quotes := recordedApple()
	f := &routeFixture{
		clk:    testkit.NewClock(clock.Real{}.Now().Truncate(time.Second)),
		quotes: map[multichain.ListingID]recordedQuote{},
		inputs: map[multichain.ListingID]money.Micros{},
	}
	quotes[aaplxSolana] = recordedQuote{err: errs.New(errs.CodeUpstreamUnavailable, "test")}
	for _, name := range []string{aaplxSolana, aaplonEth, aaplcBase} {
		f.listings = append(f.listings, byName[name])
		f.quotes[byName[name].ID] = quotes[name]
	}
	quoter := fixtureQuoter{f: f}
	r := app.NewRouter(f, app.Venues{multichain.VenueJupiter: quoter, multichain.VenueUniswap: quoter}, f, f, f.clk,
		testRules())
	stock, amount := stockID(t, g), money.MicrosFromUint64(100_000_000)

	if _, err := r.Best(t.Context(), stock, multichain.SideBuy, amount); errs.CodeOf(err) != errs.CodeQuoteUnavailable {
		t.Fatalf("Best err = %v, want quote_unavailable while Solana cannot be quoted", err)
	}
	c, err := r.BestOn(t.Context(), stock, multichain.SideBuy, amount, chain.NetworkBase)
	if err != nil || c.Winner != byName[aaplcBase].ID || len(c.Ranked) != 1 || len(c.Unavailable) != 0 {
		t.Fatalf("BestOn(base) = %+v, %v, want only the Base listing", c, err)
	}
	if _, err := r.BestOn(t.Context(), stock, multichain.SideBuy, amount, chain.NetworkArbitrum); errs.CodeOf(err) !=
		errs.CodeNoRoute {
		t.Fatalf("BestOn(arbitrum) err = %v, want no_route with no Arbitrum listing", err)
	}
}
