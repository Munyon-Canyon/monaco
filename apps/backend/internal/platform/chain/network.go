package chain

import (
	"log/slog"
	"slices"
	"strconv"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Network string

const (
	NetworkSolana    Network = "solana"
	NetworkBase      Network = "base"
	NetworkArbitrum  Network = "arbitrum"
	NetworkEthereum  Network = "ethereum"
	NetworkRobinhood Network = "robinhood"
)

type CashAsset struct {
	Symbol   string
	Address  string
	Decimals uint8
}

type networkSpec struct {
	evmChainID int64
	caip2      string
	cash       CashAsset
}

func specOf(n Network) (networkSpec, bool) {
	switch n {
	case NetworkSolana:
		return networkSpec{
			caip2: "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp",
			cash:  CashAsset{Symbol: "USDC", Address: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", Decimals: 6},
		}, true
	case NetworkBase:
		return evmSpec(8453, "USDC", "0x833589fcd6edb6e08f4c7c32d4f71b54bda02913"), true
	case NetworkArbitrum:
		return evmSpec(42161, "USDC", "0xaf88d065e77c8cc2239327c5edb3a432268e5831"), true
	case NetworkEthereum:
		return evmSpec(1, "USDC", "0xa0b86991c6218b36c1d19d4a2e9eb0ce3606eb48"), true
	case NetworkRobinhood:
		return evmSpec(4663, "USDG", "0x5fc5360d0400a0fd4f2af552add042d716f1d168"), true
	}
	return networkSpec{}, false
}

func evmSpec(chainID int64, symbol, address string) networkSpec {
	return networkSpec{
		evmChainID: chainID,
		caip2:      "eip155:" + strconv.FormatInt(chainID, 10),
		cash:       CashAsset{Symbol: symbol, Address: address, Decimals: 6},
	}
}

func Networks() []Network {
	return []Network{NetworkSolana, NetworkBase, NetworkArbitrum, NetworkEthereum, NetworkRobinhood}
}

func TradingNetworks() []Network {
	return []Network{NetworkSolana, NetworkBase, NetworkRobinhood}
}

func (n Network) Trades() bool { return slices.Contains(TradingNetworks(), n) }

func ParseNetwork(raw string) (Network, error) {
	if raw == "" {
		return NetworkSolana, nil
	}
	if _, ok := specOf(Network(raw)); ok {
		return Network(raw), nil
	}
	return "", errs.New(errs.CodeInvalidNetwork, "chain.ParseNetwork", slog.String("network", raw))
}

func (n Network) spec() networkSpec {
	s, _ := specOf(n)
	return s
}

func (n Network) IsEVM() bool { return n.spec().evmChainID != 0 }

func (n Network) EVMChainID() int64 { return n.spec().evmChainID }

func (n Network) CAIP2() string { return n.spec().caip2 }

func (n Network) PrivyChain() string { return string(n) }

func (n Network) Cash() CashAsset { return n.spec().cash }
