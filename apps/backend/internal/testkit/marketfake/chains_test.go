package marketfake_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestChainsQuoteEveryListingAtOnePriceAndCountTheQuotes(t *testing.T) {
	t.Parallel()
	c := marketfake.NewChains(testkit.NewClock(clock.Real{}.Now()), chain.NetworkSolana, chain.NetworkBase)
	got, err := c.Router().
		Best(t.Context(), multichain.StockID{}, multichain.SideBuy, money.MicrosFromUint64(5_000_000))
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Ranked) != 2 || got.Ranked[0].Network != chain.NetworkSolana || got.Ranked[0].NetShares != 20_000_000 ||
		got.Ranked[1].Venue != multichain.VenueUniswap || got.Ranked[1].NetShares != 20_000_000 {
		t.Fatalf("comparison = %+v, want 0.02 shares on Solana then Base", got.Ranked)
	}
	quoted := c.Quoted()
	slices.Sort(quoted)
	if !slices.Equal(quoted, []chain.Network{chain.NetworkBase, chain.NetworkSolana}) {
		t.Fatalf("quoted = %v", quoted)
	}
}
