package domain_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

func held(place domain.HoldingPlace, network chain.Network, token string) domain.HeldAsset {
	return domain.HeldAsset{Place: place, Network: network, Token: token}
}

func TestPlaceAsset_sortsLedgerAssetsByChainAndCash(t *testing.T) {
	t.Parallel()
	const usdc = chain.SolanaAddress("EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v")
	for asset, want := range map[string]domain.HeldAsset{
		string(usdc): held(domain.SolanaCash, chain.NetworkSolana, string(usdc)),
		"XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp":        held(domain.SolanaToken, chain.NetworkSolana, "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp"),
		"base:" + chain.NetworkBase.Cash().Address:           held(domain.EVMCash, chain.NetworkBase, chain.NetworkBase.Cash().Address),
		"robinhood:" + chain.NetworkRobinhood.Cash().Address: held(domain.EVMCash, chain.NetworkRobinhood, chain.NetworkRobinhood.Cash().Address),
		"arbitrum:0xb2":  held(domain.EVMToken, chain.NetworkArbitrum, "0xb2"),
		"dogechain:0xb2": held(domain.SolanaToken, chain.NetworkSolana, "dogechain:0xb2"),
		"solana:0xb2":    held(domain.SolanaToken, chain.NetworkSolana, "solana:0xb2"),
	} {
		if got := domain.PlaceAsset(asset, usdc); got != want {
			t.Fatalf("PlaceAsset(%q) = %+v, want %+v", asset, got, want)
		}
	}
}
