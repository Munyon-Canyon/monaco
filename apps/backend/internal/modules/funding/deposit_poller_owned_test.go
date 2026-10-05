package funding_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
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

func TestDepositPollerSkipsSignaturesTheBackendOwns(t *testing.T) {
	t.Parallel()
	owner := signatureOwner(func(sig chain.Signature) (bool, error) { return sig == "payout", nil })
	rpc, count, err := tickOwned(t, owner)
	if err != nil {
		t.Fatal(err)
	}
	if count != 0 || rpc.transferCalls != 0 {
		t.Fatalf("payout deposits = %d, transfer reads = %d, want 0 and 0", count, rpc.transferCalls)
	}
}

func TestDepositPollerStopsWhenOwnershipIsUnknown(t *testing.T) {
	t.Parallel()
	owner := signatureOwner(func(chain.Signature) (bool, error) {
		return false, errs.New(errs.CodeInternal, "test.OwnsSignature")
	})
	rpc, count, err := tickOwned(t, owner)
	if errs.CodeOf(err) != errs.CodeInternal || count != 0 || rpc.transferCalls != 0 {
		t.Fatalf("Tick = %v, deposits = %d, transfer reads = %d", err, count, rpc.transferCalls)
	}
}

func tickOwned(t *testing.T, owner signatureOwner) (*depositRPC, int, error) {
	t.Helper()
	pool := testkit.DB(t)
	user := testkit.SeedUser(t, pool, testkit.UserOpts{WithWallet: true})
	now := clock.Real{}.Now().UTC().Truncate(time.Microsecond)
	tip := solana.SignatureInfo{Signature: "tip", Slot: 1, BlockTime: depositBlockTime()}
	payout := solana.SignatureInfo{Signature: "payout", Slot: 2, BlockTime: depositBlockTime()}
	rpc := &depositRPC{
		signaturesFor: func(_ chain.Signature, until chain.Signature, _ int) []solana.SignatureInfo {
			if until == "tip" {
				return []solana.SignatureInfo{payout}
			}
			return []solana.SignatureInfo{tip}
		},
		transfers: []solana.Transfer{{
			Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6},
			Net:  money.NewBaseUnits(1_000_000, 6),
		}},
	}
	p := app.NewDepositPoller(
		pool, db.New(pool, testkit.NewIDs(72), testkit.NewClock(now)), testkit.NewIDs(73), testkit.NewClock(now),
		fakes.NewIdentity(nil, []identity.MemberWallet{{UserID: user.ID, Address: user.Address}}),
		rpc, testkit.USDCMint, app.DepositPollInterval, unlimited(), &hints{},
	).SkipOwned(owner)
	ctx := observability.WithActor(t.Context(), "system:poller.funding.deposits")
	if _, err := p.Tick(ctx); err != nil {
		t.Fatalf("bootstrap Tick = %v", err)
	}
	_, tickErr := p.Tick(ctx)
	var count int
	err := pool.QueryRow(ctx, `SELECT count(*) FROM deposits WHERE tx_signature = 'payout'`).Scan(&count)
	if err != nil {
		t.Fatal(err)
	}
	return rpc, count, tickErr
}
