package treasury_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/testkit/marketfake"
)

func TestSolanaPositions_readsTheSolanaHoldingsOfACabalThatAlsoHoldsOnEVM(t *testing.T) {
	t.Parallel()
	p := newPotCase(t)
	p.hold(t, marketfake.TSLAx().Mint.String(), 73_000_000, 50_000_000)
	p.hold(t, "base:"+chain.NetworkBase.Cash().Address, 7_000_000, 7_000_000)
	p.hold(t, "base:0xb200000000000000000000000000000000000d04", 500_000_000_000_000_000, 15_000_000)

	got, err := p.q.SolanaPositions(t.Context(), p.cabal)
	if err != nil {
		t.Fatalf("Positions over EVM rows = %v, want the Solana positions and no decode error", err)
	}
	mints := map[chain.SolanaAddress]uint64{}
	for _, position := range got {
		mints[position.Mint] = position.Units.Uint64()
	}
	if len(got) != 2 || mints[marketfake.TSLAx().Mint.Address()] != 73_000_000 || mints[usdcMint] != 100_000_000 {
		t.Fatalf("Positions = %+v, want Solana USDC and TSLAx only", got)
	}
}

func TestEVMPositions_readsOnlyTheEVMTokenRowsOfACabalAndSkipsCash(t *testing.T) {
	t.Parallel()
	p := newPotCase(t)
	p.hold(t, marketfake.TSLAx().Mint.String(), 73_000_000, 50_000_000)
	p.hold(t, "base:"+chain.NetworkBase.Cash().Address, 7_000_000, 7_000_000)
	p.hold(t, "base:0xb200000000000000000000000000000000000d04", 500_000_000_000_000_000, 15_000_000)
	p.hold(t, "robinhood:0xb200000000000000000000000000000000000d05", 305_000_000, 15_000_000)

	got, err := p.q.EVMPositions(t.Context(), p.cabal)
	if err != nil {
		t.Fatalf("EVMPositions = %v, want the EVM token rows", err)
	}
	units := map[string]string{}
	for _, position := range got {
		units[position.Key] = position.Units.String()
	}
	if len(got) != 2 || units["base:0xb200000000000000000000000000000000000d04"] != "500000000000000000" ||
		units["robinhood:0xb200000000000000000000000000000000000d05"] != "305000000" {
		t.Fatalf("EVMPositions = %+v, want the two EVM token rows and no Solana or cash row", got)
	}
}
