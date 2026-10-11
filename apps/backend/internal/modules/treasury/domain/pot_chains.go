package domain

import (
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
)

type HoldingPlace int

const (
	SolanaCash HoldingPlace = iota + 1
	SolanaToken
	EVMCash
	EVMToken
)

type HeldAsset struct {
	Place   HoldingPlace
	Network chain.Network
	Token   string
}

func PlaceAsset(asset string, usdc chain.SolanaAddress) HeldAsset {
	if asset == string(usdc) {
		return HeldAsset{Place: SolanaCash, Network: chain.NetworkSolana, Token: asset}
	}
	network, token, ok := strings.Cut(asset, ":")
	n, err := chain.ParseNetwork(network)
	if !ok || err != nil || !n.IsEVM() {
		return HeldAsset{Place: SolanaToken, Network: chain.NetworkSolana, Token: asset}
	}
	if token == n.Cash().Address {
		return HeldAsset{Place: EVMCash, Network: n, Token: token}
	}
	return HeldAsset{Place: EVMToken, Network: n, Token: token}
}
