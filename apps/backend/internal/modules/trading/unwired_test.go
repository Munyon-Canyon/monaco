package trading_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func TestUnwired_everyPortFailsClosedWithARetryableCode(t *testing.T) {
	t.Parallel()
	ctx, cabal, proposal := t.Context(), ids.CabalID{}, ids.ProposalID{}
	_, statusErr := app.UnwiredCabals{}.Status(ctx, cabal)
	_, slippageErr := app.UnwiredCabals{}.SlippageBps(ctx, cabal)
	_, walletErr := app.UnwiredCabals{}.TreasuryWallet(ctx, cabal)
	_, memberErr := app.UnwiredCabals{}.IsMember(ctx, cabal, ids.UserID{})
	_, pauseErr := app.UnwiredPauses{}.IsPaused(ctx, cabal)
	_, proposalErr := app.UnwiredProposals{}.Status(ctx, proposal)
	for i, err := range []error{statusErr, slippageErr, walletErr, memberErr, pauseErr, proposalErr} {
		if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || errs.VerdictFor(errs.CodeOf(err)) != errs.VerdictNak {
			t.Errorf("port %d err = %v, want upstream_unavailable, which naks", i, err)
		}
	}
}
