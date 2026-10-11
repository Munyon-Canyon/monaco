package multichain_test

import (
	"context"
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func pinQuote(n chain.Network) multichain.Quote {
	return multichain.Quote{ListingID: multichain.ListingIDFrom(uuid.New()), Network: n}
}

func listingIDs(quotes []multichain.Quote) []multichain.ListingID {
	out := make([]multichain.ListingID, len(quotes))
	for i, q := range quotes {
		out[i] = q.ListingID
	}
	return out
}

func TestPin_picksTheNetworksBestListingAndKeepsTheRanking(t *testing.T) {
	t.Parallel()
	ranked := []multichain.Quote{
		pinQuote(chain.NetworkSolana), pinQuote(chain.NetworkBase),
		pinQuote(chain.NetworkEthereum), pinQuote(chain.NetworkBase),
	}
	c := multichain.Comparison{Ranked: slices.Clone(ranked), Winner: ranked[0].ListingID}
	got, ok := c.Pin(chain.NetworkBase)
	if !ok || got.Winner != ranked[1].ListingID {
		t.Fatalf("Pin(base) = %s, %v, want %s, true", got.Winner, ok, ranked[1].ListingID)
	}
	if !slices.Equal(listingIDs(got.Ranked), listingIDs(ranked)) || c.Winner != ranked[0].ListingID {
		t.Fatalf("Pin changed the ranking or the original: %+v, winner %s", got.Ranked, c.Winner)
	}
	if _, ok := c.Pin(chain.NetworkArbitrum); ok {
		t.Fatal("Pin(arbitrum) = true with no arbitrum quote")
	}
}

type bestOnly struct{ winner multichain.ListingID }

func (r bestOnly) Best(
	context.Context, multichain.StockID, multichain.Side, money.Micros,
) (multichain.Comparison, error) {
	return multichain.Comparison{Winner: r.winner}, nil
}

type networkScoped struct {
	bestOnly
	asked chain.Network
}

func (r *networkScoped) BestOn(
	_ context.Context, _ multichain.StockID, _ multichain.Side, _ money.Micros, n chain.Network,
) (multichain.Comparison, error) {
	r.asked = n
	return multichain.Comparison{Winner: multichain.ListingIDFrom(uuid.Nil)}, nil
}

func TestBestOn_asksARouterThatScopesByNetworkAndFallsBackToBest(t *testing.T) {
	t.Parallel()
	best := multichain.ListingIDFrom(uuid.New())
	scoped := &networkScoped{bestOnly: bestOnly{winner: best}}
	got, err := multichain.BestOn(t.Context(), scoped, multichain.StockID{}, multichain.SideBuy, money.Micros{},
		chain.NetworkBase)
	if err != nil || !got.Winner.IsZero() || scoped.asked != chain.NetworkBase {
		t.Fatalf("BestOn = %s, %v, asked %q, want the network-scoped answer for base", got.Winner, err, scoped.asked)
	}
	got, err = multichain.BestOn(t.Context(), bestOnly{winner: best}, multichain.StockID{}, multichain.SideBuy,
		money.Micros{}, chain.NetworkBase)
	if err != nil || got.Winner != best {
		t.Fatalf("BestOn = %s, %v, want Best's %s from a router with no network scope", got.Winner, err, best)
	}
}
