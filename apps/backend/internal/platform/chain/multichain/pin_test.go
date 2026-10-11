package multichain_test

import (
	"slices"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
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
