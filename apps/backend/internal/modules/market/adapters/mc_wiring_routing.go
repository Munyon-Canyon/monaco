package adapters

import (
	"context"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/jupiterquote"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/adapters/uniswapquote"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const routerQuoteAge = 15 * time.Second

func routingClient(d module.Deps, name string, opts ...httpclient.Option) *httpclient.Client {
	if d.HTTPClient == nil {
		return httpclient.New(name, opts...)
	}
	return d.HTTPClient(name, opts...)
}

type DeferredQuoter struct {
	once   sync.Once
	build  func() app.Venues
	venues app.Venues
}

func (q *DeferredQuoter) Quote(
	ctx context.Context, l multichain.Listing, side multichain.Side, amount money.Micros,
) (multichain.Quote, error) {
	q.once.Do(func() { q.venues = q.build() })
	return q.venues.Quote(ctx, l, side, amount)
}

type DeferredRouter struct {
	once   sync.Once
	build  func() *app.Router
	router *app.Router
}

func DeferRouter(build func() *app.Router) *DeferredRouter { return &DeferredRouter{build: build} }

func (r *DeferredRouter) Best(
	ctx context.Context, stock multichain.StockID, side multichain.Side, amount money.Micros,
) (multichain.Comparison, error) {
	r.once.Do(func() { r.router = r.build() })
	return r.router.Best(ctx, stock, side, amount)
}

func MultichainRoutingQuoter(d module.Deps) *DeferredQuoter {
	return &DeferredQuoter{build: func() app.Venues { return RoutingVenues(d) }}
}

func RoutingVenues(d module.Deps) app.Venues {
	return routingVenues(d, app.NewSimulationAccounts(d.Pool))
}

func routingVenues(d module.Deps, accounts uniswapquote.Accounts) app.Venues {
	venues := app.Venues{multichain.VenueJupiter: jupiterquote.NewQuoter(d.JupiterClient(), d.Clock)}
	cfg := d.Config.Routing
	if cfg.UniswapAPIKey != "" {
		client := routingClient(d, "uniswap", uniswapquote.Options(cfg.UniswapBaseURL, cfg.UniswapQuoteTimeout)...)
		venues[multichain.VenueUniswap] = uniswapquote.New(client, cfg.UniswapAPIKey, accounts, d.Clock).
			WithLimiter(d.UniswapLimiter()).WithWrapper(uniswapquote.NewWrapper(wrapperRPC(d), wrapperDelegate()))
	}
	return venues
}

func wrapperDelegate() chain.EVMAddress {
	a, _ := chain.ParseEVMAddress(uniswapquote.DefaultDelegate)
	return a
}

func wrapperRPC(d module.Deps) map[chain.Network]*httpclient.Client {
	rpc := map[chain.Network]*httpclient.Client{}
	for _, n := range chain.Networks() {
		if url := d.Config.EVM.RPCURL(n); n.IsEVM() && url != "" {
			rpc[n] = routingClient(d, "evm-rpc-wrapper-"+string(n),
				httpclient.WithBaseURL(url), httpclient.WithTimeout(d.Config.Timeouts.RPC))
		}
	}
	return rpc
}

func MultichainRoutingRouter(d module.Deps, prices app.SampledPrices) *app.Router {
	cfg := d.Config
	bridge := NewPrivyBridgeQuotes(routingClient(d, "privy-bridge",
		httpclient.WithBaseURL(cfg.Privy.BaseURL), httpclient.WithTimeout(cfg.Timeouts.Privy)),
		cfg.Privy.AppID, cfg.Privy.AppSecret)
	accounts := app.NewSimulationAccounts(d.Pool)
	router := app.NewRouter(NewStockListings(d.Pool), routingVenues(d, accounts), app.NewStockPrices(prices, d.Clock),
		bridge, d.Clock, routerRules(cfg.Routing))
	if cfg.Routing.UniswapAPIKey == "" {
		return router
	}
	return router.WithSimulationCheck(app.NewSimulationCheck(accounts, NewEVMCode(d), wrapperDelegate()))
}

func routerRules(cfg config.Routing) app.RouterRules {
	return app.RouterRules{
		MinEdgeBps: uint64(max(cfg.MinEdgeBps, 0)), MaxImpactBps: uint64(max(cfg.MaxImpactBps, 0)),
		QuoteTimeout: cfg.UniswapQuoteTimeout, MaxQuoteAge: routerQuoteAge,
	}
}
