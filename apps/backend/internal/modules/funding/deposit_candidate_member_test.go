package funding_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const memberSignature chain.Signature = "member-to-member-signature"

func memberResolver(f candidateFixture, usdc ...uint64) app.DepositCandidateResolver {
	transfers := make([]solana.Transfer, 0, len(usdc))
	for _, amount := range usdc {
		transfers = append(transfers, solana.Transfer{
			Mint: chain.Mint{Address: testkit.USDCMint, Decimals: 6}, Net: money.NewBaseUnits(amount, 6),
		})
	}
	deps := module.Deps{
		Config: testkit.Config(), Pool: f.pool, IDs: testkit.NewIDs(310), Clock: clock.Real{},
		UoW: f.uow,
	}
	return app.NewDepositCandidateResolver(
		candidateRPC{transfers: transfers}, testkit.USDCMint,
		[]port.SignatureOwner{treasury.New(deps).SignatureOwner()},
		app.NewCreditDepositHandler(f.uow, &hints{}), f.ids,
	)
}

func (f candidateFixture) postedTxn(t *testing.T, user testkit.SeededUser, kind string) {
	t.Helper()
	if _, err := f.pool.Exec(t.Context(), `INSERT INTO user_txns (id, user_id, kind, status, tx_signature, created_at)
		VALUES ($1, $2, $3, 'settled', $4, $5)`, f.ids.NewV7(), user.ID.UUID(), kind, string(memberSignature), f.now); err != nil {
		t.Fatal(err)
	}
}

func (f candidateFixture) resolveFor(
	t *testing.T, resolver app.DepositCandidateResolver, user testkit.SeededUser,
) app.DepositCandidateResolution {
	t.Helper()
	f.record(t, app.DepositCandidate{
		Signature: memberSignature, Wallet: user.Address, UserID: user.ID, Slot: 42,
		BlockTime: f.now.Add(-time.Minute), Source: "poller",
	})
	e := events.DepositCandidateSeen{
		V: 1, CandidateID: f.ids.NewV7(), UserID: user.ID.UUID(), WalletAddress: user.Address,
		TxSignature: memberSignature, Slot: 42, BlockTime: ptr(f.now.Add(-time.Minute)), Source: "poller",
	}
	resolution, err := resolver.Fetch(t.Context(), e)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.uow.Do(t.Context(), func(ctx context.Context, tx db.Tx) error {
		return resolver.Apply(ctx, tx, e, resolution, f.now)
	}); err != nil {
		t.Fatal(err)
	}
	return resolution
}

func (f candidateFixture) credits(t *testing.T, user testkit.SeededUser) int {
	t.Helper()
	return f.count(t, `SELECT count(*) FROM deposits WHERE tx_signature = $1 AND user_id = $2`,
		string(memberSignature), user.ID.UUID())
}

func TestDepositCandidateResolver_aMemberToMemberWithdrawalCreditsTheReceiver(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	sender := testkit.SeedUser(t, f.pool, testkit.UserOpts{WithWallet: true})
	f.postedTxn(t, sender, "withdrawal")
	resolver := memberResolver(f, 5_000_000)

	if got := f.resolveFor(t, resolver, f.user); got.Ours || got.Amount != money.MicrosFromUint64(5_000_000) {
		t.Fatalf("receiver resolution = %+v, want a 5 USDC deposit that is not ours", got)
	}
	if n := f.credits(t, f.user); n != 1 {
		t.Fatalf("receiver deposits = %d, want 1", n)
	}
	if status := f.candidateStatus(t, memberSignature); status != "credited" {
		t.Fatalf("candidate status = %q, want credited", status)
	}
}

func TestDepositCandidateResolver_aMembersOwnOutflowStaysOurs(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	f.postedTxn(t, f.user, "withdrawal")

	if got := f.resolveFor(t, memberResolver(f, 5_000_000), f.user); !got.Ours {
		t.Fatalf("own outflow resolution = %+v, want ours", got)
	}
	if n := f.credits(t, f.user); n != 0 {
		t.Fatalf("own outflow deposits = %d, want 0", n)
	}
}

func TestDepositCandidateResolver_oneTransactionPayingTwoMembersCreditsBothOnce(t *testing.T) {
	t.Parallel()
	f := newCandidateFixture(t)
	second := testkit.SeedUser(t, f.pool, testkit.UserOpts{WithWallet: true})
	resolver := memberResolver(f, 5_000_000)

	if got := f.resolveFor(t, resolver, f.user); got.Ours {
		t.Fatalf("first member resolution = %+v, want a deposit", got)
	}
	f.postedTxn(t, f.user, "deposit")
	if got := f.resolveFor(t, resolver, second); got.Ours {
		t.Fatalf("second member resolution = %+v, want a deposit after the first was credited", got)
	}
	if first, other := f.credits(t, f.user), f.credits(t, second); first != 1 || other != 1 {
		t.Fatalf("deposits = %d and %d, want one each", first, other)
	}
}
