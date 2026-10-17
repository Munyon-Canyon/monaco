package chain_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func TestParseNetwork(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		raw     string
		want    chain.Network
		caip2   string
		cash    string
		chainID int64
	}{
		{"", chain.NetworkSolana, "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", "USDC", 0},
		{"solana", chain.NetworkSolana, "solana:5eykt4UsFv8P8NJdTREpY1vzqKqZKvdp", "USDC", 0},
		{"base", chain.NetworkBase, "eip155:8453", "USDC", 8453},
		{"arbitrum", chain.NetworkArbitrum, "eip155:42161", "USDC", 42161},
		{"ethereum", chain.NetworkEthereum, "eip155:1", "USDC", 1},
		{"robinhood", chain.NetworkRobinhood, "eip155:4663", "USDG", 4663},
	} {
		got, err := chain.ParseNetwork(tc.raw)
		if err != nil || got != tc.want {
			t.Fatalf("ParseNetwork(%q) = %q, %v, want %q", tc.raw, got, err, tc.want)
		}
		if got.IsEVM() != (tc.chainID != 0) || got.CAIP2() != tc.caip2 || got.EVMChainID() != tc.chainID {
			t.Errorf("%s: IsEVM %v CAIP2 %q chain %d", got, got.IsEVM(), got.CAIP2(), got.EVMChainID())
		}
		if got.PrivyChain() != string(tc.want) || got.Cash().Symbol != tc.cash || got.Cash().Decimals != 6 {
			t.Errorf("%s: PrivyChain %q Cash %+v", got, got.PrivyChain(), got.Cash())
		}
	}
}

func TestParseNetworkRejectsUnknownNetworks(t *testing.T) {
	t.Parallel()
	for _, raw := range []string{"Solana", "polygon", "eip155:8453", " base"} {
		if _, err := chain.ParseNetwork(raw); errs.CodeOf(err) != errs.CodeInvalidNetwork {
			t.Errorf("ParseNetwork(%q) error = %v, want invalid_network", raw, err)
		}
	}
}

func TestCashAddressesParseOnTheirNetwork(t *testing.T) {
	t.Parallel()
	if len(chain.Networks()) != 5 {
		t.Fatalf("Networks() = %v", chain.Networks())
	}
	for _, n := range chain.Networks() {
		addr := n.Cash().Address
		if !n.IsEVM() {
			if _, err := chain.ParseAddress(addr); err != nil {
				t.Errorf("%s: cash address %q is not a Solana address: %v", n, addr, err)
			}
			continue
		}
		if a, err := chain.ParseEVMAddress(addr); err != nil || a.String() != addr {
			t.Errorf("%s: cash address %q is not a lowercase EVM address: %v", n, addr, err)
		}
	}
}

func TestTradingNetworksExcludeTheDepositOnlyChains(t *testing.T) {
	t.Parallel()
	want := []chain.Network{chain.NetworkSolana, chain.NetworkBase, chain.NetworkRobinhood}
	if got := chain.TradingNetworks(); !slices.Equal(got, want) {
		t.Fatalf("TradingNetworks() = %v, want %v", got, want)
	}
	for _, n := range chain.Networks() {
		if got := n.Trades(); got != slices.Contains(want, n) {
			t.Errorf("%s.Trades() = %v", n, got)
		}
	}
	if chain.NetworkArbitrum.Trades() || chain.NetworkEthereum.Trades() {
		t.Fatal("arbitrum and ethereum must not trade")
	}
}
