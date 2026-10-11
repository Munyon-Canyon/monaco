package trading_test

import (
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (e *engineEnv) listing(t *testing.T) multichain.ListingID {
	t.Helper()
	var id multichain.ListingID
	if err := id.UnmarshalText([]byte(e.ids.NewV7().String())); err != nil {
		t.Fatal(err)
	}
	token := "0x" + strings.ReplaceAll(id.UUID().String(), "-", "")[:32] + "00000000"
	if _, err := e.pool.Exec(t.Context(), `INSERT INTO asset_listings
		(id, issuer, symbol, network, chain_id, token_address, decimals, shares_per_token, venue, enabled, created_at)
		VALUES ($1, 'coinbase', 'AAPLc', 'base', 8453, $2, 8, 1, 'uniswap', false, $3)`,
		id.UUID(), token, e.clk.Now()); err != nil {
		t.Fatal(err)
	}
	return id
}

func (e *engineEnv) comparison(t *testing.T, winner multichain.ListingID) *multichain.Comparison {
	t.Helper()
	var stock multichain.StockID
	if err := stock.UnmarshalText([]byte(e.ids.NewV7().String())); err != nil {
		t.Fatal(err)
	}
	at := e.clk.Now()
	return &multichain.Comparison{
		StockID: stock, Side: multichain.SideBuy, Winner: winner,
		Ranked: []multichain.Quote{{
			ListingID: winner, Network: chain.NetworkBase, Venue: multichain.VenueUniswap, Side: multichain.SideBuy,
			InputMicros: money.MicrosFromUint64(buyMicros), NetShares: 306_000_000,
			NetMicros: money.MicrosFromUint64(buyMicros), QuotedAt: at, ExpiresAt: at.Add(30 * time.Second),
			RouteID: "route-base",
		}},
	}
}

func (e *engineEnv) routeOf(t *testing.T, swap uuid.UUID) (uuid.UUID, string, bool) {
	t.Helper()
	var winner uuid.UUID
	var route string
	err := e.pool.QueryRow(t.Context(),
		`SELECT winner_listing_id, comparison->'ranked'->0->>'route_id' FROM trade_routes WHERE trade_id = $1`, swap).
		Scan(&winner, &route)
	if err != nil && strings.Contains(err.Error(), "no rows") {
		return uuid.Nil, "", false
	}
	if err != nil {
		t.Fatal(err)
	}
	return winner, route, true
}

func TestExecuteTrade_recordsThePassedComparisonWithTheSwap(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	winner := e.listing(t)
	cmd := e.buy()
	cmd.Route = e.comparison(t, winner)

	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
		t.Fatal(err)
	}
	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 1 {
		t.Fatalf("%d swaps, want 1", len(swaps))
	}
	got, route, ok := e.routeOf(t, swaps[0])
	if !ok || got != winner.UUID() || route != "route-base" {
		t.Fatalf("route %v %s %q, want the comparison with winner %s", ok, got, route, winner.UUID())
	}
}

func TestExecuteTrade_aTradeWithoutAComparisonRecordsNoRoute(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	cmd := e.buy()

	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
		t.Fatal(err)
	}
	if _, _, ok := e.routeOf(t, e.swapsOf(t, cmd.ProposalID)[0]); ok {
		t.Fatal("a route was recorded for a trade that passed without a comparison")
	}
}

func TestExecuteTrade_aRetryCarriesTheOriginalSwapsRoute(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	winner := e.listing(t)
	cmd := e.buy()
	cmd.Route = e.comparison(t, winner)
	e.jup.FailOnce("Order", errs.New(errs.CodeJupiterRejected, "jupiter.Order"))
	if err := e.handle(t, e.delivery(t, cmd), cmd); err != nil {
		t.Fatal(err)
	}
	first := e.swapsOf(t, cmd.ProposalID)[0]

	retry := e.buy()
	retry.ProposalID = cmd.ProposalID
	retry.Retry = &app.Retry{Of: ids.SwapIDFrom(first), SlippageBps: 100}
	if err := e.handle(t, e.delivery(t, retry), retry); err != nil {
		t.Fatal(err)
	}
	swaps := e.swapsOf(t, cmd.ProposalID)
	if len(swaps) != 2 {
		t.Fatalf("%d swaps, want the failed one and its retry", len(swaps))
	}
	for _, s := range swaps {
		if got, route, ok := e.routeOf(t, s); !ok || got != winner.UUID() || route != "route-base" {
			t.Fatalf("swap %s route %v %s %q, want the original comparison", s, ok, got, route)
		}
	}
}

func TestExecuteTrade_aComparisonNamingAnUnknownListingCreatesNoSwap(t *testing.T) {
	t.Parallel()
	e := newEngineEnv(t)
	var unknown multichain.ListingID
	if err := unknown.UnmarshalText([]byte(e.ids.NewV7().String())); err != nil {
		t.Fatal(err)
	}
	cmd := e.buy()
	cmd.Route = e.comparison(t, unknown)

	if err := e.handle(t, e.delivery(t, cmd), cmd); err == nil {
		t.Fatal("err = nil, want the route insert to fail")
	}
	if n := len(e.swapsOf(t, cmd.ProposalID)); n != 0 {
		t.Fatalf("%d swaps, want none: the swap and its route commit together", n)
	}
}
