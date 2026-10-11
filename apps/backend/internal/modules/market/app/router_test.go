package app_test

import (
	"context"
	"errors"
	"maps"
	"math/big"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type recordedQuote struct {
	big     *big.Int
	units   int64
	gas     uint64
	poolFee uint64
	age     time.Duration
	ttl     time.Duration
	err     error
}

type routeFixture struct {
	mu          sync.Mutex
	listingsErr error
	priceErr    error
	bridged     func(money.Micros) (money.Micros, error)
	clk         *testkit.Clock
	logs        testkit.Logs
	listings    []multichain.Listing
	quotes      map[multichain.ListingID]recordedQuote
	inputs      map[multichain.ListingID]money.Micros
}

func (f *routeFixture) EnabledListings(context.Context, multichain.StockID) ([]multichain.Listing, error) {
	return f.listings, f.listingsErr
}

func (f *routeFixture) SharePrice(context.Context, multichain.StockID) (money.Micros, error) {
	return money.MicrosFromUint64(336_844_000), f.priceErr
}

func (f *routeFixture) BridgeOut(_ context.Context, _ chain.Network, amount money.Micros) (money.Micros, error) {
	if f.bridged != nil {
		return f.bridged(amount)
	}
	return money.MicrosFromUint64(amount.Uint64() - amount.Uint64()*75/100_000), nil
}

type fixtureQuoter struct{ f *routeFixture }

func (q fixtureQuoter) Quote(
	_ context.Context, l multichain.Listing, side multichain.Side, amount money.Micros,
) (multichain.Quote, error) {
	r := q.f.quotes[l.ID]
	if r.err != nil {
		return multichain.Quote{}, r.err
	}
	q.f.mu.Lock()
	q.f.inputs[l.ID] = amount
	q.f.mu.Unlock()
	at := q.f.clk.Now().Add(-r.age)
	ttl := cmpOr(r.ttl, 30*time.Second)
	var costs []multichain.Cost
	if r.gas > 0 {
		costs = append(costs, multichain.Cost{Kind: multichain.CostGas, Micros: money.MicrosFromUint64(r.gas)})
	}
	if r.poolFee > 0 {
		costs = append(costs, multichain.Cost{Kind: multichain.CostPoolFee, Micros: money.MicrosFromUint64(r.poolFee)})
	}
	return multichain.Quote{
		Side: side, InputMicros: amount, GrossOutput: cmpBig(r.big, big.NewInt(r.units)), Costs: costs,
		QuotedAt: at, ExpiresAt: at.Add(ttl), RouteID: "recorded",
	}, nil
}

func cmpBig(v, fallback *big.Int) *big.Int {
	if v != nil {
		return v
	}
	return fallback
}

func cmpOr(d, fallback time.Duration) time.Duration {
	if d == 0 {
		return fallback
	}
	return d
}

func listingID(t *testing.T, g *testkit.IDs) multichain.ListingID {
	t.Helper()
	var id multichain.ListingID
	if err := id.UnmarshalText([]byte(g.NewV7().String())); err != nil {
		t.Fatal(err)
	}
	return id
}

func stockID(t *testing.T, g *testkit.IDs) multichain.StockID {
	t.Helper()
	var id multichain.StockID
	if err := id.UnmarshalText([]byte(g.NewV7().String())); err != nil {
		t.Fatal(err)
	}
	return id
}

const (
	aaplxSolana  = "aaplx-solana"
	aaplonSolana = "aaplon-solana"
	aaplonEth    = "aaplon-ethereum"
	aaplcBase    = "aaplc-base"
	wholeSolana  = "whole-solana"
)

func appleListings(t *testing.T, g *testkit.IDs) map[string]multichain.Listing {
	t.Helper()
	rat := func(s string) *big.Rat { r, _ := new(big.Rat).SetString(s); return r }
	return map[string]multichain.Listing{
		aaplxSolana: {
			ID: listingID(t, g), Issuer: "xstocks", Symbol: "AAPLx", Network: chain.NetworkSolana,
			Decimals: 8, SharesPerToken: rat("1.0032690125398187"), Venue: multichain.VenueJupiter,
		},
		aaplonSolana: {
			ID: listingID(t, g), Issuer: "ondo", Symbol: "AAPLon", Network: chain.NetworkSolana,
			Decimals: 9, SharesPerToken: rat("1.003376073740221058"), Venue: multichain.VenueJupiter,
		},
		aaplonEth: {
			ID: listingID(t, g), Issuer: "ondo", Symbol: "AAPLon", Network: chain.NetworkEthereum,
			Decimals: 18, SharesPerToken: rat("1.003376073740221058"), Venue: multichain.VenueUniswap,
		},
		aaplcBase: {
			ID: listingID(t, g), Issuer: "coinbase", Symbol: "AAPLc", Network: chain.NetworkBase,
			Decimals: 8, SharesPerToken: rat("1"), Venue: multichain.VenueUniswap,
		},
		wholeSolana: {
			ID: listingID(t, g), Issuer: "xstocks", Symbol: "WHLx", Network: chain.NetworkSolana,
			Decimals: 8, SharesPerToken: rat("1"), Venue: multichain.VenueJupiter,
		},
	}
}

func recordedApple() map[string]recordedQuote {
	return map[string]recordedQuote{
		aaplxSolana:  {units: 29_610_838},
		aaplonSolana: {units: 295_449_773},
		aaplonEth:    {units: 294_521_447_600_868_269, gas: 34_015, poolFee: 999_250},
		aaplcBase:    {units: 30_690_575, gas: 76_119, poolFee: 9_993},
	}
}

func route(
	t *testing.T, quotes map[string]recordedQuote, rules app.RouterRules, tweaks ...func(*routeFixture),
) (multichain.Comparison, map[string]multichain.Listing, *routeFixture, error) {
	t.Helper()
	g := testkit.NewIDs(4969)
	byName := appleListings(t, g)
	f := &routeFixture{
		clk: testkit.NewClock(
			clock.Real{}.Now().Truncate(time.Second),
		),
		quotes: map[multichain.ListingID]recordedQuote{},
		inputs: map[multichain.ListingID]money.Micros{},
	}
	for name, q := range quotes {
		f.listings = append(f.listings, byName[name])
		f.quotes[byName[name].ID] = q
	}
	for _, tweak := range tweaks {
		tweak(f)
	}
	quoter := fixtureQuoter{f: f}
	r := app.NewRouter(
		f,
		app.Venues{multichain.VenueJupiter: quoter, multichain.VenueUniswap: quoter},
		f,
		f,
		f.clk,
		rules,
	)
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, &f.logs))
	c, err := r.Best(ctx, stockID(t, g), multichain.SideBuy, money.MicrosFromUint64(100_000_000))
	return c, byName, f, err
}

func costsOf(q multichain.Quote) map[multichain.CostKind]uint64 {
	out := map[multichain.CostKind]uint64{}
	for _, cost := range q.Costs {
		out[cost.Kind] += cost.Micros.Uint64()
	}
	return out
}

func TestRouterRanksEveryListingInUnderlyingShares(t *testing.T) {
	t.Parallel()
	c, l, _, err := route(t, recordedApple(), testRules())
	if err != nil {
		t.Fatal(err)
	}
	if c.Winner != l[aaplcBase].ID {
		t.Fatalf("winner = %s, want Base", c.Winner)
	}
	want := []string{aaplcBase, aaplxSolana, aaplonSolana, aaplonEth}
	if len(c.Ranked) != len(want) {
		t.Fatalf("ranked = %+v", c.Ranked)
	}
	for i, name := range want {
		if c.Ranked[i].ListingID != l[name].ID || c.Ranked[i].Issuer != l[name].Issuer {
			t.Fatalf("ranked[%d] = %s %s, want %s", i, c.Ranked[i].Issuer, c.Ranked[i].ListingID, name)
		}
	}
}

func TestRouterChargesTheBridgeAndGasOfAnEVMRoute(t *testing.T) {
	t.Parallel()
	c, l, f, err := route(t, recordedApple(), testRules())
	if err != nil {
		t.Fatal(err)
	}
	base := c.Ranked[0]
	if got := f.inputs[l[aaplcBase].ID]; got.Uint64() != 99_925_000 {
		t.Fatalf("Base swap input = %s, want 99925000 after the 7.5 bps bridge", got)
	}
	costs := costsOf(base)
	if costs[multichain.CostBridge] != 75_000 || costs[multichain.CostGas] != 76_119 {
		t.Fatalf("Base costs = %+v", costs)
	}
	if base.NetShares != 306_671_961 || base.InputMicros.Uint64() != 100_000_000 {
		t.Fatalf("Base net shares = %d input = %s", base.NetShares, base.InputMicros)
	}
	if sol := c.Ranked[1]; sol.NetShares != 297_076_362 || len(sol.Costs) != 0 {
		t.Fatalf("AAPLx Solana = %d shares, costs %+v", sol.NetShares, sol.Costs)
	}
	eth := costsOf(c.Ranked[3])
	if eth[multichain.CostBridge] != 75_000 || eth[multichain.CostPoolFee] != 999_250 ||
		eth[multichain.CostPriceImpact] != 0 {
		t.Fatalf("Ondo Ethereum = %+v, want the 1%% pool fee to explain its gap to the reference", eth)
	}
}

func TestRouterKeepsSolanaUnlessAnEVMRouteBeatsItByTheEdge(t *testing.T) {
	t.Parallel()
	quotes := recordedApple()
	delete(quotes, aaplonSolana)
	quotes[aaplcBase] = recordedQuote{units: 29_720_000}
	c, l, _, err := route(t, quotes, testRules())
	if err != nil {
		t.Fatal(err)
	}
	if c.Winner != l[aaplxSolana].ID || c.Ranked[0].ListingID != l[aaplxSolana].ID {
		t.Fatalf("winner = %s, want Solana when Base is ahead by under 10 bps: %+v", c.Winner, c.Ranked)
	}
	if c.Ranked[1].ListingID != l[aaplcBase].ID || c.Ranked[1].NetShares <= c.Ranked[0].NetShares {
		t.Fatalf("Base should still rank by shares behind the kept Solana winner: %+v", c.Ranked)
	}
	quotes[aaplcBase] = recordedQuote{units: 29_800_000}
	c, l, _, err = route(t, quotes, testRules())
	if err != nil {
		t.Fatal(err)
	}
	if c.Winner != l[aaplcBase].ID {
		t.Fatalf("winner = %s, want Base once it clears the edge", c.Winner)
	}
}

func TestRouterDropsExpiredStaleAndOverImpactQuotes(t *testing.T) {
	t.Parallel()
	tests := map[string]recordedQuote{
		"expired":     {units: 40_000_000, ttl: time.Second, age: 2 * time.Second},
		"stale":       {units: 40_000_000, age: 16 * time.Second},
		"over impact": {units: 28_000_000},
	}
	for name, base := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			quotes := recordedApple()
			quotes[aaplcBase] = base
			c, l, _, err := route(t, quotes, testRules())
			if err != nil {
				t.Fatal(err)
			}
			for _, q := range c.Ranked {
				if q.ListingID == l[aaplcBase].ID {
					t.Fatalf("Base quote kept: %+v", q)
				}
			}
			if len(c.Ranked) != 3 || c.Winner != l[aaplxSolana].ID {
				t.Fatalf("comparison = %+v", c)
			}
		})
	}
}

func TestRouterFailsWithNoRouteWhenNoListingQuotes(t *testing.T) {
	t.Parallel()
	_, _, _, err := route(t, map[string]recordedQuote{
		aaplxSolana: {err: errs.New(errs.CodeNoRoute, "test")},
		aaplcBase:   {err: errs.New(errs.CodeNoRoute, "test")},
	}, testRules())
	if errs.CodeOf(err) != errs.CodeNoRoute {
		t.Fatalf("err = %v, want no_route", err)
	}
}

func TestRouterRefusesSells(t *testing.T) {
	t.Parallel()
	r := app.NewRouter(nil, nil, nil, nil, testkit.NewClock(clock.Real{}.Now()), testRules())
	_, err := r.Best(t.Context(), multichain.StockID{}, multichain.SideSell, money.MicrosFromUint64(1))
	if errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("err = %v, want invalid_input", err)
	}
}

func TestRouterChargesImpactBeyondThePoolFee(t *testing.T) {
	t.Parallel()
	quotes := map[string]recordedQuote{
		aaplxSolana: {units: 29_610_838},
		aaplcBase:   {units: 29_500_000, poolFee: 10_000},
	}
	c, l, _, err := route(t, quotes, testRules())
	if err != nil {
		t.Fatal(err)
	}
	for _, q := range c.Ranked {
		if q.ListingID != l[aaplcBase].ID {
			continue
		}
		for _, cost := range q.Costs {
			if cost.Kind == multichain.CostPriceImpact && cost.Micros.Uint64() == 546_020 {
				return
			}
		}
		t.Fatalf("Base costs = %+v, want 546020 micros of impact past the pool fee", q.Costs)
	}
	t.Fatalf("Base missing from %+v", c.Ranked)
}

func TestRouterDropsEVMListingsItCannotPrice(t *testing.T) {
	t.Parallel()
	failing := errs.New(errs.CodeUpstreamUnavailable, "test")
	cases := map[string]func(*routeFixture){
		"no reference price": func(f *routeFixture) { f.priceErr = failing },
		"bridge down": func(f *routeFixture) {
			f.bridged = func(money.Micros) (money.Micros, error) { return money.Micros{}, failing }
		},
		"bridge returns nothing": func(f *routeFixture) {
			f.bridged = func(money.Micros) (money.Micros, error) { return money.Micros{}, nil }
		},
		"bridge returns more": func(f *routeFixture) {
			f.bridged = func(a money.Micros) (money.Micros, error) { return money.MicrosFromUint64(a.Uint64() + 1), nil }
		},
	}
	for name, tweak := range cases {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, l, _, err := route(t, recordedApple(), testRules(), tweak)
			if err != nil {
				t.Fatal(err)
			}
			if len(c.Ranked) != 2 || c.Winner != l[aaplxSolana].ID {
				t.Fatalf("comparison = %+v, want only the two Solana listings", c.Ranked)
			}
		})
	}
}

func TestRouterFailsClosedWhenTheSolanaListingCannotBeQuoted(t *testing.T) {
	t.Parallel()
	for name, solana := range map[string]recordedQuote{
		"quote errors": {err: errs.New(errs.CodeUpstreamUnavailable, "test")},
		"quote stale":  {units: 29_610_838, age: 16 * time.Second},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, l, f, err := route(t, map[string]recordedQuote{
				aaplonEth: recordedApple()[aaplonEth], aaplcBase: recordedApple()[aaplcBase], aaplxSolana: solana,
			}, testRules())
			if errs.CodeOf(err) != errs.CodeQuoteUnavailable {
				t.Fatalf("comparison = %+v, %v, want quote_unavailable instead of an EVM winner", c, err)
			}
			if got := f.logs.Lines("market.router.solana_unquoted", "venue"); len(got) != 1 || got[0] != "jupiter" {
				t.Fatalf("solana_unquoted logs = %v, want one for jupiter", got)
			}
			dropped := f.logs.Lines("market.router.venue_dropped", "listing_id")
			if len(dropped) != 1 || dropped[0] != l[aaplxSolana].ID.String() {
				t.Fatalf("venue_dropped logs = %v, want the Solana listing once", dropped)
			}
		})
	}
}

func TestRouterLogsAnUnpricedQuoteWithNoOutput(t *testing.T) {
	t.Parallel()
	failing := errs.New(errs.CodeUpstreamUnavailable, "test")
	c, l, f, err := route(t, map[string]recordedQuote{
		aaplxSolana: {units: 29_610_838}, aaplonSolana: {units: 0},
	}, testRules(), func(f *routeFixture) { f.priceErr = failing })
	if err != nil || len(c.Ranked) != 1 || c.Winner != l[aaplxSolana].ID {
		t.Fatalf("comparison = %+v, %v, want only AAPLx", c.Ranked, err)
	}
	want := []string{l[aaplonSolana].ID.String() + " no_output"}
	if got := f.logs.Lines("market.router.venue_dropped", "listing_id", "reason"); !slices.Equal(got, want) {
		t.Fatalf("venue_dropped = %v, want %v", got, want)
	}
}

func TestRouterRoutesToTheBestEVMListingWhenTheStockHasNoSolanaListing(t *testing.T) {
	t.Parallel()
	c, l, f, err := route(t, map[string]recordedQuote{
		aaplonEth: recordedApple()[aaplonEth], aaplcBase: recordedApple()[aaplcBase],
	}, testRules())
	if err != nil || c.Winner != l[aaplcBase].ID || len(c.Ranked) != 2 {
		t.Fatalf("comparison = %+v, %v", c, err)
	}
	if got := f.logs.Lines("market.router.solana_unquoted"); len(got) != 0 {
		t.Fatalf("solana_unquoted logged %v for a stock with no Solana listing", got)
	}
}

func TestRouterLogsEachDroppedVenueWithItsReason(t *testing.T) {
	t.Parallel()
	_, l, f, err := route(t, map[string]recordedQuote{
		aaplxSolana:  {units: 29_610_838},
		aaplonSolana: {units: 295_449_773, age: 16 * time.Second},
		aaplonEth:    {units: 294_521_447_600_868_269, ttl: time.Second, age: 2 * time.Second},
		aaplcBase:    {err: errs.New(errs.CodeNoRoute, "test")},
	}, testRules())
	if err != nil {
		t.Fatal(err)
	}
	got := map[string]string{}
	for _, line := range f.logs.Lines("market.router.venue_dropped", "listing_id", "reason") {
		id, reason, _ := strings.Cut(line, " ")
		if _, dup := got[id]; dup {
			t.Fatalf("listing %s logged twice", id)
		}
		got[id] = reason
	}
	want := map[string]string{
		l[aaplonSolana].ID.String(): "stale", l[aaplonEth].ID.String(): "expired",
		l[aaplcBase].ID.String(): "no_route",
	}
	if !maps.Equal(got, want) {
		t.Fatalf("venue_dropped = %v, want %v", got, want)
	}
}

func TestRouterDropsQuotesWithNoUsableOutput(t *testing.T) {
	t.Parallel()
	huge, _ := new(big.Int).SetString("1000000000000000000000000000000000000000", 10)
	c, l, _, err := route(t, map[string]recordedQuote{
		aaplxSolana: {units: 29_610_838}, aaplonSolana: {units: 0}, aaplcBase: {big: huge},
	}, testRules())
	if err != nil || len(c.Ranked) != 1 || c.Winner != l[aaplxSolana].ID {
		t.Fatalf("comparison = %+v, %v, want only AAPLx", c.Ranked, err)
	}
}

func TestRouterSurfacesListingAndCancellationErrors(t *testing.T) {
	t.Parallel()
	failing := errs.New(errs.CodeUpstreamUnavailable, "test")
	_, _, _, err := route(
		t,
		recordedApple(),
		testRules(),
		func(f *routeFixture) { f.listingsErr = failing },
	)
	if !errors.Is(err, failing) {
		t.Fatalf("err = %v, want the listing error", err)
	}
	g := testkit.NewIDs(4972)
	f := &routeFixture{
		clk: testkit.NewClock(clock.Real{}.Now()), quotes: map[multichain.ListingID]recordedQuote{},
		inputs: map[multichain.ListingID]money.Micros{},
	}
	f.listings = []multichain.Listing{appleListings(t, g)[aaplxSolana]}
	r := app.NewRouter(f, fixtureQuoter{f: f}, f, f, f.clk, testRules())
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	if _, err := r.Best(
		ctx,
		stockID(t, g),
		multichain.SideBuy,
		money.MicrosFromUint64(1),
	); errs.CodeOf(
		err,
	) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal for a canceled fan-out", err)
	}
}

func TestVenuesRefuseAListingOnAnUnknownVenue(t *testing.T) {
	t.Parallel()
	l := multichain.Listing{Venue: "kyber"}
	if _, err := (app.Venues{}).Quote(
		t.Context(),
		l,
		multichain.SideBuy,
		money.MicrosFromUint64(1),
	); errs.CodeOf(
		err,
	) != errs.CodeNoRoute {
		t.Fatalf("err = %v, want no_route", err)
	}
}

func TestRouterKeepsSolanaWhenAnEVMQuoteIsTooLargeToCompare(t *testing.T) {
	t.Parallel()
	c, l, _, err := route(t, map[string]recordedQuote{
		aaplxSolana: {units: 29_610_838}, aaplcBase: {units: 200_000_000_000_000},
	}, testRules())
	if err != nil || c.Winner != l[aaplxSolana].ID {
		t.Fatalf("comparison = %+v, %v, want Solana kept over an overflowing quote", c.Ranked, err)
	}
}

func testRules() app.RouterRules {
	return app.RouterRules{
		MinEdgeBps:   10,
		MaxImpactBps: 300,
		QuoteTimeout: 5 * time.Second,
		MaxQuoteAge:  15 * time.Second,
	}
}

func TestRouterLetsAnEVMRouteWinAtExactlyTheEdgeAndKeepsSolanaOnATie(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		base   int64
		winner string
	}{
		"exactly 10 bps ahead": {100_100_000, aaplcBase},
		"just under the edge":  {100_099_999, wholeSolana},
		"a tie":                {100_000_000, wholeSolana},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			c, l, _, err := route(t, map[string]recordedQuote{
				wholeSolana: {units: 100_000_000}, aaplcBase: {units: tc.base},
			}, testRules())
			if err != nil || c.Winner != l[tc.winner].ID || c.Ranked[0].ListingID != l[tc.winner].ID {
				t.Fatalf("comparison = %+v, %v, want %s first", c.Ranked, err, tc.winner)
			}
		})
	}
}
