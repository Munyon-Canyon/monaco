package config

import "github.com/monaco/monaco/apps/backend/internal/platform/chain"

type EVM struct {
	RPCURLBase      string
	RPCURLArbitrum  string
	RPCURLEthereum  string
	RPCURLRobinhood string
	LogsMaxBlocks   int32
}

func (e EVM) RPCURL(n chain.Network) string {
	switch n {
	case chain.NetworkBase:
		return e.RPCURLBase
	case chain.NetworkArbitrum:
		return e.RPCURLArbitrum
	case chain.NetworkEthereum:
		return e.RPCURLEthereum
	case chain.NetworkRobinhood:
		return e.RPCURLRobinhood
	case chain.NetworkSolana:
	}
	return ""
}

func evmFields() []field {
	return []field{
		text("EVM_RPC_URL_BASE", "https://mainnet.base.org",
			func(c *Config) *string { return &c.EVM.RPCURLBase }).secret(),
		text("EVM_RPC_URL_ARBITRUM", "https://arb1.arbitrum.io/rpc",
			func(c *Config) *string { return &c.EVM.RPCURLArbitrum }).secret(),
		text("EVM_RPC_URL_ETHEREUM", "https://ethereum-rpc.publicnode.com",
			func(c *Config) *string { return &c.EVM.RPCURLEthereum }).secret(),
		text("EVM_RPC_URL_ROBINHOOD", "https://rpc.mainnet.chain.robinhood.com",
			func(c *Config) *string { return &c.EVM.RPCURLRobinhood }).secret(),
		count("EVM_LOGS_MAX_BLOCKS", 10, func(c *Config) *int32 { return &c.EVM.LogsMaxBlocks }),
	}
}
