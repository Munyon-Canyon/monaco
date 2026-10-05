package funding_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestBounceChain_BuildsNoClientUntilFirstUse(t *testing.T) {
	t.Parallel()
	funding.New(module.Deps{Config: testkit.Config()}).Consumers()
	c := newFlow08(t).funding.BounceChain()
	if mint, err := c.MintConfig(t.Context(), testkit.USDCMint); err != nil || mint.Mint.Decimals != 6 {
		t.Fatalf("MintConfig = %+v, %v, want the 6 decimal mint", mint, err)
	}
	sigs := []chain.Signature{flow08USDCSig, flow08DustSig, flow08StockSig, flow08OtherSig}
	if statuses, err := c.SignatureStatuses(t.Context(), sigs); err != nil || len(statuses) != len(sigs) {
		t.Fatalf("SignatureStatuses = %+v, %v, want one per signature", statuses, err)
	}
	_, accounts, err := c.Accounts(t.Context(), []chain.SolanaAddress{flow08Sender}, 0)
	if err != nil || len(accounts) != 1 || accounts[0].Exists {
		t.Fatalf("Accounts = %+v, %v, want the sender's account read as closed", accounts, err)
	}
}
