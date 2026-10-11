package trading_test

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
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
	_, proposalErr := app.UnwiredProposals{}.Retryable(ctx, proposal)
	for i, err := range []error{statusErr, slippageErr, walletErr, memberErr, pauseErr, proposalErr} {
		if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || errs.VerdictFor(errs.CodeOf(err)) != errs.VerdictNak {
			t.Errorf("port %d err = %v, want upstream_unavailable, which naks", i, err)
		}
	}
}

func TestUnwired_theMultichainExecutionPortsAreNotImplementedYet(t *testing.T) {
	t.Parallel()
	ctx := t.Context()
	_, listingErr := app.UnwiredListings{}.Listing(ctx, multichain.ListingID{})
	_, assetErr := app.UnwiredListings{}.ListingByAsset(ctx, "base:0x")
	_, buildErr := app.UnwiredEVMVenue{}.Build(ctx, app.EVMSwapSpec{})
	_, fillErr := app.UnwiredEVMVenue{}.Fill(ctx, chain.NetworkBase, "0xhash", chain.EVMAddress{}, chain.EVMAddress{})
	for i, err := range []error{listingErr, assetErr, buildErr, fillErr} {
		if errs.CodeOf(err) != errs.CodeNotImplemented || errs.VerdictFor(errs.CodeOf(err)) == errs.VerdictNak {
			t.Errorf("port %d err = %v, want not_implemented, which never naks", i, err)
		}
	}
}

func TestEVMSend_passesTheIdempotencyKeyToTheWalletOnASponsoredSend(t *testing.T) {
	t.Parallel()
	wallets := &recordingWallets{}
	tx := multichain.EVMTransaction{Network: chain.NetworkBase, Data: []byte{1}}

	hash, err := adapters.EVMSend{Wallets: wallets}.Send(t.Context(), "evm-wallet", tx, "trade:swap")

	want := tx
	want.IdempotencyKey = "trade:swap"
	if err != nil || hash != "0xsent1" || len(wallets.sent) != 1 || wallets.sent[0].wallet != "evm-wallet" ||
		!wallets.sent[0].sponsor || wallets.sent[0].tx.IdempotencyKey != want.IdempotencyKey ||
		wallets.sent[0].tx.Network != want.Network {
		t.Fatalf("Send = %q, %v, sent %+v, want one sponsored send carrying the key", hash, err, wallets.sent)
	}
}
