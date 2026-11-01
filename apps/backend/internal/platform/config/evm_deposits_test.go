package config_test

import (
	"slices"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func TestLoad_evmDepositNetworksDefaultToEveryUSDCEVMChain(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(required())
	if err != nil {
		t.Fatal(err)
	}
	want := []chain.Network{chain.NetworkBase, chain.NetworkArbitrum, chain.NetworkEthereum}
	if !slices.Equal(cfg.Funding.EVMDepositNetworks, want) {
		t.Fatalf("EVMDepositNetworks = %v, want %v", cfg.Funding.EVMDepositNetworks, want)
	}
}

func TestLoad_evmDepositNetworksNarrowToTheConfiguredList(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(append(required(), "EVM_DEPOSIT_NETWORKS=base"))
	if err != nil {
		t.Fatal(err)
	}
	if want := []chain.Network{chain.NetworkBase}; !slices.Equal(cfg.Funding.EVMDepositNetworks, want) {
		t.Fatalf("EVMDepositNetworks = %v, want %v", cfg.Funding.EVMDepositNetworks, want)
	}
}

func TestLoad_evmDepositNetworksKeepTheConfiguredOrder(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(append(required(), "EVM_DEPOSIT_NETWORKS=ethereum,base,arbitrum"))
	if err != nil {
		t.Fatal(err)
	}
	want := []chain.Network{chain.NetworkEthereum, chain.NetworkBase, chain.NetworkArbitrum}
	if !slices.Equal(cfg.Funding.EVMDepositNetworks, want) {
		t.Fatalf("EVMDepositNetworks = %v, want %v", cfg.Funding.EVMDepositNetworks, want)
	}
	if got := cfg.Redacted()["EVM_DEPOSIT_NETWORKS"]; got != "ethereum,base,arbitrum" {
		t.Fatalf("Redacted EVM_DEPOSIT_NETWORKS = %q, want the list as set", got)
	}
}

func TestLoad_refusesADepositNetworkThatIsNotAUSDCEVMChainOrRepeats(t *testing.T) {
	t.Parallel()
	for _, bad := range []string{"solana", "robinhood", "polygon", "base,", ",base", "base,base", "base, arbitrum"} {
		_, err := config.Load(append(required(), "EVM_DEPOSIT_NETWORKS="+bad))
		want := "config.Load: invalid_input: invalid EVM_DEPOSIT_NETWORKS " +
			"(comma-separated distinct EVM networks taking USDC: base, arbitrum or ethereum, or none)"
		if err == nil || err.Error() != want {
			t.Errorf("EVM_DEPOSIT_NETWORKS=%s: Load error = %v, want %q", bad, err, want)
		}
	}
}

func TestLoad_evmDepositNetworksNoneOffersNoEVMDeposit(t *testing.T) {
	t.Parallel()
	cfg, err := config.Load(append(required(), "EVM_DEPOSIT_NETWORKS=none"))
	if err != nil {
		t.Fatal(err)
	}
	if len(cfg.Funding.EVMDepositNetworks) != 0 {
		t.Fatalf("EVMDepositNetworks = %v, want none", cfg.Funding.EVMDepositNetworks)
	}
	if got := cfg.Redacted()["EVM_DEPOSIT_NETWORKS"]; got != "none" {
		t.Fatalf("Redacted EVM_DEPOSIT_NETWORKS = %q, want none", got)
	}
}
