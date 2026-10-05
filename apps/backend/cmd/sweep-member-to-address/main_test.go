package main

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
)

func TestMain(m *testing.M) {
	testkit.Main(m, testkit.WithChild(main))
}

func wallet(id string) chain.Wallet {
	return chain.Wallet{ID: id, Address: chainfake.WalletAddress(id), HasAppSigner: true}
}

var (
	dest   = chainfake.WalletAddress("destination")
	member = wallet("wallet-member")
	cabal  = wallet("wallet-treasury")
	stray  = wallet("wallet-privy-only")
	usdc   = chain.Mint{Address: "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v", Decimals: 6}
	tables = fakeTables{members: []chain.Wallet{member}, treasuries: []chain.Wallet{cabal}}
)
