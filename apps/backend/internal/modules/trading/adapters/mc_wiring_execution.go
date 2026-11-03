package adapters

import (
	"net/http"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/privy"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
)

func MultichainExecutionPrivy(cfg config.Config, clk clock.Clock) (*privy.Client, error) {
	return privy.New(cfg, clk)
}

func MultichainExecutionVenue(d module.Deps) (*EVMVenue, bool) {
	cfg := d.Config
	if cfg.Routing.UniswapAPIKey == "" {
		return nil, false
	}
	uniswap := clients(d)("uniswap-swap",
		httpclient.WithBaseURL(cfg.Routing.UniswapBaseURL), httpclient.WithTimeout(cfg.Routing.UniswapTimeout))
	return NewEVMVenue(uniswap, cfg.Routing.UniswapAPIKey, EVMRPC(d), d.Clock).WithLimiter(d.UniswapLimiter()), true
}

func clients(d module.Deps) func(string, ...httpclient.Option) *httpclient.Client {
	if d.HTTPClient == nil {
		return httpclient.New
	}
	return d.HTTPClient
}

func EVMRPC(d module.Deps) map[chain.Network]*httpclient.Client {
	client, cfg := clients(d), d.Config
	rpc := map[chain.Network]*httpclient.Client{}
	for _, n := range chain.Networks() {
		if url := cfg.EVM.RPCURL(n); n.IsEVM() && url != "" {
			rpc[n] = client("evm-rpc-"+string(n), httpclient.WithBaseURL(url), httpclient.WithTimeout(cfg.Timeouts.RPC),
				httpclient.WithReturned(http.StatusTooManyRequests))
		}
	}
	return rpc
}
