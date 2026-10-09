package funding_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

type signatureOwner func(chain.Signature) (bool, error)

func (f signatureOwner) OwnsSignature(_ context.Context, sig chain.Signature) (bool, error) {
	return f(sig)
}

func TestDepositPollerRecordsSignaturesTheBackendOwnsAndTheResolverMarksThemOurs(t *testing.T) {
	t.Parallel()
	owner := signatureOwner(func(sig chain.Signature) (bool, error) { return sig == "payout", nil })
	f, delivery := tickOwned(t, owner)
	if delivery.outcome != bus.OutcomeAck {
		t.Fatalf("delivery = %q, want ack", delivery.outcome)
	}
	if got := f.candidateStatus(t, "payout"); got != "ours" {
		t.Fatalf("payout candidate = %q, want ours", got)
	}
	if got := f.count(t, `SELECT count(*) FROM deposits WHERE tx_signature = 'payout'`); got != 0 {
		t.Fatalf("payout deposits = %d, want 0", got)
	}
}

func TestDepositPollerCandidateStaysPendingWhenOwnershipIsUnknown(t *testing.T) {
	t.Parallel()
	owner := signatureOwner(func(chain.Signature) (bool, error) {
		return false, errs.New(errs.CodeInternal, "test.OwnsSignature")
	})
	f, delivery := tickOwned(t, owner)
	if delivery.outcome == bus.OutcomeAck {
		t.Fatal("delivery acked while ownership was unknown")
	}
	if got := f.candidateStatus(t, "payout"); got != "pending" {
		t.Fatalf("payout candidate = %q, want pending", got)
	}
	if got := f.count(t, `SELECT count(*) FROM deposits WHERE tx_signature = 'payout'`); got != 0 {
		t.Fatalf("payout deposits = %d, want 0", got)
	}
}

func tickOwned(t *testing.T, owner signatureOwner) (candidateFixture, *candidateDelivery) {
	t.Helper()
	f := newCandidateFixture(t)
	tip := solana.SignatureInfo{Signature: "tip", Slot: 1, BlockTime: depositBlockTime()}
	payout := solana.SignatureInfo{Signature: "payout", Slot: 2, BlockTime: depositBlockTime()}
	rpc := &depositRPC{
		signaturesFor: func(_ chain.Signature, until chain.Signature, _ int) []solana.SignatureInfo {
			if until == "tip" {
				return []solana.SignatureInfo{payout}
			}
			return []solana.SignatureInfo{tip}
		},
	}
	p := app.NewDepositPoller(
		f.pool, db.New(f.pool, testkit.NewIDs(72), testkit.NewClock(f.now)),
		testkit.NewIDs(73), testkit.NewClock(f.now),
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: f.user.ID, Address: f.user.Address}}),
		rpc, testkit.USDCMint, app.DepositPollInterval, unlimited(),
	)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposit_watch")
	if _, err := p.Tick(ctx); err != nil {
		t.Fatalf("bootstrap Tick = %v", err)
	}
	if _, err := p.Tick(ctx); err != nil {
		t.Fatalf("Tick = %v", err)
	}
	d := newCandidateDispatch(t, f, candidateRPC{transfers: []solana.Transfer{{
		Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(1_000_000, 6),
	}}}, owner)
	return f, d.dispatchSeen(ctx, t)
}
