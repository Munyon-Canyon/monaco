package adapters

import (
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
	client := d.HTTPClient
	if client == nil {
		client = httpclient.New
	}
	rpc := map[chain.Network]*httpclient.Client{}
	for _, n := range chain.Networks() {
		if url := cfg.EVM.RPCURL(n); n.IsEVM() && url != "" {
			rpc[n] = client("evm-rpc-"+string(n), httpclient.WithBaseURL(url), httpclient.WithTimeout(cfg.Timeouts.RPC))
		}
	}
	uniswap := client("uniswap-swap",
		httpclient.WithBaseURL(cfg.Routing.UniswapBaseURL), httpclient.WithTimeout(cfg.Routing.UniswapTimeout))
	return NewEVMVenue(uniswap, cfg.Routing.UniswapAPIKey, rpc, d.Clock).WithLimiter(d.UniswapLimiter()), true
}
