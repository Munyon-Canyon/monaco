package treasury_test

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func TestQueries_ownsSignaturesAndReadsWalletLedger(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	q := adapters.NewQueries(f.pool, nil, nil, f.clock, chain.SolanaAddress(f.cfg.Solana.USDCMint))
	for _, sig := range []chain.Signature{"missing", "cabal-signature", "user-signature"} {
		assertSignatureOwned(f.ctx(), t, q, sig, false)
	}
	user := f.user(t)
	cabal := f.cabal(t)
	u, c, err := f.fund(user, cabal, 2_500_000, 25, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	u.TxSignature = "user-signature"
	c.TxSignature = "cabal-signature"
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
	pending, err := domain.NewUserTxn(domain.UserTxnHeader{
		ID: f.ids.NewV7(), UserID: user, Kind: domain.UserDeposit, Status: domain.TxnPending,
	}, []domain.UserEntry{
		{Account: domain.UserWallet, Asset: f.usdc(), Amount: amount(1)},
		{Account: domain.UserExternal, Asset: f.usdc(), Amount: amount(-1)},
	})
	if err != nil {
		t.Fatal(err)
	}
	if err := f.do(func(ctx context.Context, tx db.Tx) error {
		return f.ledger.PostUserTxn(ctx, tx, pending)
	}); err != nil {
		t.Fatal(err)
	}
	for _, sig := range []chain.Signature{"cabal-signature", "user-signature"} {
		assertSignatureOwned(f.ctx(), t, q, sig, true)
	}
	settled, pendingCount, err := q.WalletLedgerMicros(f.ctx(), user, chain.SolanaAddress(f.cfg.Solana.USDCMint))
	assertWalletLedger(t, settled, pendingCount, err, amount(-2_500_000), 1)
	empty, count, err := q.WalletLedgerMicros(f.ctx(), f.user(t), chain.SolanaAddress(f.cfg.Solana.USDCMint))
	assertWalletLedger(t, empty, count, err, money.SignedMicros{}, 0)
	ctx, cancel := context.WithCancel(f.ctx())
	cancel()
	_, err = q.OwnsSignature(ctx, "canceled")
	assertInternal(t, err)
	_, _, err = q.WalletLedgerMicros(ctx, user, chain.SolanaAddress(f.cfg.Solana.USDCMint))
	assertInternal(t, err)
}

func assertSignatureOwned(ctx context.Context, t *testing.T, q *adapters.Queries, sig chain.Signature, want bool) {
	t.Helper()
	owned, err := q.OwnsSignature(ctx, sig)
	if err != nil || owned != want {
		t.Fatalf("OwnsSignature(%q) = %t, %v, want %t, nil", sig, owned, err, want)
	}
}

func assertWalletLedger(
	t *testing.T, settled money.SignedMicros, pending int, err error, want money.SignedMicros, wantPending int,
) {
	t.Helper()
	if err != nil || settled != want || pending != wantPending {
		t.Fatalf("WalletLedgerMicros() = %v, %d, %v, want %v, %d, nil", settled, pending, err, want, wantPending)
	}
}

func assertInternal(t *testing.T, err error) {
	t.Helper()
	if errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("error code = %q, want %q", errs.CodeOf(err), errs.CodeInternal)
	}
}
