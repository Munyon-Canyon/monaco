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
	for _, name := range []string{aaplxSolana, aaplonRobinhood, aaplcBase} {
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
		errs.CodeInvalidNetwork {
		t.Fatalf("BestOn(arbitrum) err = %v, want invalid_network for a deposit-only network", err)
	}
}

func TestRouterQuotesARobinhoodXStocksListingAndChargesItsBridge(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(5361)
	byName := appleListings(t, g)
	robinhood := byName[aaplxSolana]
	robinhood.ID, robinhood.Network, robinhood.Decimals, robinhood.Venue = listingID(t, g), chain.NetworkRobinhood, 18,
		multichain.VenueUniswap
	f := &routeFixture{
		clk:      testkit.NewClock(clock.Real{}.Now().Truncate(time.Second)),
		listings: []multichain.Listing{byName[aaplxSolana], robinhood},
		quotes: map[multichain.ListingID]recordedQuote{
			byName[aaplxSolana].ID: recordedApple()[aaplxSolana],
			robinhood.ID:           {units: 297_500_000_000_000_000, gas: 12_000, poolFee: 2_998},
		},
		inputs: map[multichain.ListingID]money.Micros{},
	}
	quoter := fixtureQuoter{f: f}
	r := app.NewRouter(f, app.Venues{multichain.VenueJupiter: quoter, multichain.VenueUniswap: quoter}, f, f, f.clk,
		testRules())
	stock, amount := stockID(t, g), money.MicrosFromUint64(100_000_000)

	c, err := r.Best(t.Context(), stock, multichain.SideBuy, amount)
	if err != nil || c.Winner != robinhood.ID || len(c.Ranked) != 2 || c.Ranked[0].Network != chain.NetworkRobinhood {
		t.Fatalf("Best = %+v, %v, want the Robinhood AAPLx listing ranked first", c, err)
	}
	if costs := costsOf(c.Ranked[0]); costs[multichain.CostBridge] != 75_000 || costs[multichain.CostGas] != 12_000 ||
		f.bridges[chain.NetworkRobinhood] != 1 || f.inputs[robinhood.ID].Uint64() != 99_925_000 {
		t.Fatalf("Robinhood costs = %+v, bridges = %v, swap input = %s, want one 7.5 bps bridge to Robinhood",
			costs, f.bridges, f.inputs[robinhood.ID])
	}
	c, err = r.BestOn(t.Context(), stock, multichain.SideBuy, amount, chain.NetworkRobinhood)
	if err != nil || c.Winner != robinhood.ID || len(c.Ranked) != 1 {
		t.Fatalf("BestOn(robinhood) = %+v, %v, want only the Robinhood listing", c, err)
	}
}

func TestRouterNeverQuotesOrListsTheDepositOnlyNetworks(t *testing.T) {
	t.Parallel()
	g := testkit.NewIDs(5491)
	byName := appleListings(t, g)
	deposit := func(n chain.Network) multichain.Listing {
		l := byName[aaplxBase]
		l.ID, l.Network = listingID(t, g), n
		return l
	}
	arbitrum, ethereum := deposit(chain.NetworkArbitrum), deposit(chain.NetworkEthereum)
	f := &routeFixture{
		clk:      testkit.NewClock(clock.Real{}.Now().Truncate(time.Second)),
		listings: []multichain.Listing{byName[aaplxSolana], byName[aaplcBase], arbitrum, ethereum},
		quotes: map[multichain.ListingID]recordedQuote{
			byName[aaplxSolana].ID: recordedApple()[aaplxSolana],
			byName[aaplcBase].ID:   recordedApple()[aaplcBase],
			arbitrum.ID:            {units: 400_000_000},
			ethereum.ID:            {units: 400_000_000},
		},
		inputs: map[multichain.ListingID]money.Micros{},
	}
	quoter := fixtureQuoter{f: f}
	r := app.NewRouter(f, app.Venues{multichain.VenueJupiter: quoter, multichain.VenueUniswap: quoter}, f, f, f.clk,
		testRules())
	stock, amount := stockID(t, g), money.MicrosFromUint64(100_000_000)

	c, err := r.Best(t.Context(), stock, multichain.SideBuy, amount)
	if err != nil || len(c.Ranked) != 2 || len(c.Unavailable) != 0 {
		t.Fatalf("Best = %+v, %v, want only the Solana and Base listings", c, err)
	}
	for _, q := range c.Ranked {
		if !q.Network.Trades() {
			t.Fatalf("ranked %s, want trading networks only", q.Network)
		}
	}
	for _, l := range []multichain.Listing{arbitrum, ethereum} {
		if _, quoted := f.inputs[l.ID]; quoted || f.bridges[l.Network] != 0 {
			t.Fatalf("%s was quoted or bridged: inputs %v bridges %v", l.Network, f.inputs, f.bridges)
		}
		if _, err := r.BestOn(t.Context(), stock, multichain.SideBuy, amount, l.Network); errs.CodeOf(err) !=
			errs.CodeInvalidNetwork {
			t.Fatalf("BestOn(%s) err = %v, want invalid_network", l.Network, err)
		}
	}
}
