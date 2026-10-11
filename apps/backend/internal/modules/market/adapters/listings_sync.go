package adapters

import (
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/coinbaseb20"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/ondo"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/robinhood"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/xstocks"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

const issuerTimeout = 15 * time.Second

func NewListingSync(d module.Deps, xs *xstocks.Client) *app.ListingSync {
	cfg := d.Config
	if !cfg.Listings.SyncEnabled {
		return nil
	}
	issuer := func(name, baseURL string) *httpclient.Client {
		return d.HTTPClient(name, httpclient.WithBaseURL(baseURL), httpclient.WithTimeout(issuerTimeout),
			httpclient.WithRetry(3, 250*time.Millisecond, 2*time.Second))
	}
	sources := []app.ListingSource{
		xstocks.ListingFeed{Client: xs},
		ondo.New(issuer("ondo", cfg.Listings.OndoBaseURL)),
		coinbaseb20.New(issuer("coinbase", cfg.Listings.CoinbaseBaseURL)),
		robinhood.New(issuer("robinhood", cfg.Listings.RobinhoodBaseURL)),
	}
	verifier := NewListingVerifier(d.HTTPClient("evm-rpc", httpclient.WithTimeout(cfg.Timeouts.RPC)),
		cfg.EVM.RPCURL, solana.New(cfg, d.Clock))
	return app.NewListingSync(d.UoW, d.IDs, d.Clock, sources, verifier)
}
