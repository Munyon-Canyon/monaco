package app

import (
	"context"
	"encoding/json"
	"math/big"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func insertListing(t *testing.T, e *sweeperEnv, network chain.Network, token string) multichain.ListingID {
	t.Helper()
	var id multichain.ListingID
	if err := id.UnmarshalText([]byte(e.ids.NewV7().String())); err != nil {
		t.Fatal(err)
	}
	chainID := map[chain.Network]any{chain.NetworkSolana: nil, chain.NetworkBase: int64(8453)}[network]
	const q = `INSERT INTO asset_listings
		(id, issuer, symbol, network, chain_id, token_address, decimals, shares_per_token, venue, enabled, created_at)
		VALUES ($1, 'coinbase', 'AAPLc', $2, $3, $4, 8, 1, 'uniswap', false, $5)`
	if _, err := e.pool.Exec(t.Context(), q, id.UUID(), network, chainID, token, e.clk.Now()); err != nil {
		t.Fatal(err)
	}
	return id
}

func routedComparison(t *testing.T, e *sweeperEnv, winner, loser multichain.ListingID) multichain.Comparison {
	t.Helper()
	at := e.clk.Now()
	var stock multichain.StockID
	if err := stock.UnmarshalText([]byte(e.ids.NewV7().String())); err != nil {
		t.Fatal(err)
	}
	quote := func(id multichain.ListingID, n chain.Network, v multichain.Venue, shares multichain.ShareNanos) multichain.Quote {
		return multichain.Quote{
			ListingID: id, Network: n, Venue: v, Side: multichain.SideBuy,
			InputMicros: money.MicrosFromUint64(100_000_000), GrossOutput: big.NewInt(30_690_575),
			Costs:     []multichain.Cost{{Kind: multichain.CostBridge, Micros: money.MicrosFromUint64(75_000)}},
			NetShares: shares, NetMicros: money.MicrosFromUint64(100_000_000),
			QuotedAt: at, ExpiresAt: at.Add(30 * time.Second), RouteID: "route-" + string(n),
		}
	}
	return multichain.Comparison{
		StockID: stock, Side: multichain.SideBuy,
		Ranked: []multichain.Quote{
			quote(winner, chain.NetworkBase, multichain.VenueUniswap, 306_000_000),
			quote(loser, chain.NetworkSolana, multichain.VenueJupiter, 297_000_000),
		},
		Winner: winner,
	}
}

func TestRecordRouteStoresTheComparisonOncePerSwap(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	swap := e.insert(t)
	base := insertListing(t, e, chain.NetworkBase, "0x"+strings.Repeat("b2", 20))
	solana := insertListing(t, e, chain.NetworkSolana, "route-test-"+swap.ID.String())
	first := routedComparison(t, e, base, solana)
	second := routedComparison(t, e, solana, base)
	for _, c := range []multichain.Comparison{first, second} {
		err := e.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
			return RecordRoute(ctx, tx, ids.SwapIDFrom(swap.ID), c, "", e.clk.Now())
		})
		if err != nil {
			t.Fatal(err)
		}
	}
	row, err := e.q.FindTradeRoute(t.Context(), swap.ID)
	if err != nil {
		t.Fatal(err)
	}
	if row.WinnerListingID != first.Winner.UUID() {
		t.Fatalf("winner = %s, want the first record's %s", row.WinnerListingID, first.Winner)
	}
	var got multichain.Comparison
	if err := json.Unmarshal(row.Comparison, &got); err != nil {
		t.Fatal(err)
	}
	if len(got.Ranked) != 2 || got.Ranked[0].NetShares != 306_000_000 || got.Ranked[0].Network != chain.NetworkBase {
		t.Fatalf("stored comparison = %+v", got)
	}
}

func TestRecordRouteRefusesAComparisonWithNoWinnerListing(t *testing.T) {
	t.Parallel()
	e := newSweeperEnv(t)
	swap := e.insert(t)
	c := routedComparison(t, e, insertListing(t, e, chain.NetworkBase, "0x"+strings.Repeat("c3", 20)),
		insertListing(t, e, chain.NetworkSolana, "route-missing-"+swap.ID.String()))
	c.Winner = multichain.ListingID{}
	err := e.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return RecordRoute(ctx, tx, ids.SwapIDFrom(swap.ID), c, "", e.clk.Now())
	})
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("err = %v, want internal", err)
	}
	if _, err := e.q.FindTradeRoute(t.Context(), swap.ID); err == nil {
		t.Fatal("a refused route was stored")
	}
}
