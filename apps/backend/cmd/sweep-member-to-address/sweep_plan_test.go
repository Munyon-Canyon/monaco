package main

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestPlanSweep(t *testing.T) {
	t.Parallel()
	payer := chain.SolanaAddress("9ixcyg5nNxGCJLtSyJYibP7EgQBw4BfpNLbDe7GQ14eh")
	one := money.NewBaseUnits(1_000_000, 6)
	zero := money.NewBaseUnits(0, 6)
	src := func(w chain.Wallet) sweepSource { return sweepSource{kind: kindMember, wallet: w} }
	for name, tc := range map[string]struct {
		src     sweepSource
		balance money.BaseUnits
		dryRun  bool
		want    verdict
	}{
		"live funded":              {src(member), one, false, verdictSweep},
		"dry-run funded":           {src(member), one, true, verdictWouldSweep},
		"zero balance":             {src(member), zero, false, verdictSkipZero},
		"source is destination":    {src(chain.Wallet{ID: "w", Address: dest}), one, false, verdictSkipDestination},
		"source is the fee payer":  {src(chain.Wallet{Address: payer}), one, false, verdictSkipRelayer},
		"unknown wallet":           {src(chain.Wallet{Address: stray.Address}), one, false, verdictNoPrivyWallet},
		"unknown wallet, dry-run":  {src(chain.Wallet{Address: stray.Address}), one, true, verdictNoPrivyWallet},
		"unknown wallet, no money": {src(chain.Wallet{Address: stray.Address}), zero, false, verdictSkipZero},
	} {
		if got := planSweep(tc.src, tc.balance, dest, payer, tc.dryRun); got != tc.want {
			t.Errorf("%s: planSweep = %q, want %q", name, got, tc.want)
		}
	}
}
