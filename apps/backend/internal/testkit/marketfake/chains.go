package marketfake

import (
	"context"
	"math/big"
	"slices"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	chainSharePriceMicros = 250_000_000
	chainTokenDecimals    = 8
)

type Chains struct {
	clock    clock.Clock
	listings []multichain.Listing
	mu       sync.Mutex
	quoted   []chain.Network
}

func NewChains(clk clock.Clock, networks ...chain.Network) *Chains {
	g := testkit.NewIDs(5364)
	c := &Chains{clock: clk}
	for _, n := range networks {
		venue := multichain.VenueUniswap
		if n == chain.NetworkSolana {
			venue = multichain.VenueJupiter
		}
		c.listings = append(c.listings, multichain.Listing{
			ID: multichain.ListingIDFrom(g.NewV7()), Issuer: "xstocks", Symbol: "AAPLx", Network: n,
			Decimals: chainTokenDecimals, SharesPerToken: big.NewRat(1, 1), Venue: venue, Enabled: true,
		})
	}
	return c
}

func (c *Chains) Router() *app.Router {
	return app.NewRouter(c, c, c, c, c.clock, app.RouterRules{
		MinEdgeBps: 10, MaxImpactBps: 300, QuoteTimeout: 5 * time.Second, MaxQuoteAge: 15 * time.Second,
	})
}

func (c *Chains) Quoted() []chain.Network {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]chain.Network(nil), c.quoted...)
}

func (c *Chains) EnabledListings(context.Context, multichain.StockID) ([]multichain.Listing, error) {
	return slices.Clone(c.listings), nil
}

func (c *Chains) SharePrice(context.Context, multichain.StockID) (money.Micros, error) {
	return money.MicrosFromUint64(chainSharePriceMicros), nil
}

func (c *Chains) BridgeOut(_ context.Context, _ chain.Network, amount money.Micros) (money.Micros, error) {
	return amount, nil
}

func (c *Chains) Quote(
	_ context.Context, l multichain.Listing, side multichain.Side, amount money.Micros,
) (multichain.Quote, error) {
	c.mu.Lock()
	c.quoted = append(c.quoted, l.Network)
	c.mu.Unlock()
	units := new(big.Int).Mul(new(big.Int).SetUint64(amount.Uint64()), big.NewInt(100_000_000))
	units.Quo(units, big.NewInt(chainSharePriceMicros))
	at := c.clock.Now()
	return multichain.Quote{
		Side: side, InputMicros: amount, GrossOutput: units, QuotedAt: at, ExpiresAt: at.Add(30 * time.Second),
	}, nil
}
