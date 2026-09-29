package chainfake_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/relayer"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chainfake"
)

var _ relayer.Signer = (*chainfake.Signer)(nil)

func TestWallets_reuseAndIdempotentCreates(t *testing.T) {
	t.Parallel()
	var w chainfake.Wallets
	first, _ := w.FindOrCreateMemberWallet(t.Context(), "did:privy:a")
	again, _ := w.FindOrCreateMemberWallet(t.Context(), "did:privy:a")
	app, _ := w.CreateAppWallet(t.Context(), "k")
	appAgain, _ := w.CreateAppWallet(t.Context(), "k")
	if first != again || app != appAgain || w.Creates() != 2 {
		t.Fatalf("wallets %v %v %v %v after %d creates", first, again, app, appAgain, w.Creates())
	}
	w.FailOnce("CreateAppWallet", errs.New(errs.CodePrivyUnavailable, "test"))
	if _, err := w.CreateAppWallet(t.Context(), "k2"); errs.CodeOf(err) != errs.CodePrivyUnavailable {
		t.Fatalf("scripted fault = %v", err)
	}
	w.Fail("FindOrCreateMemberWallet", errs.New(errs.CodePrivyUnavailable, "test"))
	if _, err := w.FindOrCreateMemberWallet(t.Context(), "did:privy:a"); err == nil {
		t.Fatal("Fail did not stick")
	}
}

func TestSigner_signsAsTheWallet(t *testing.T) {
	t.Parallel()
	var w chainfake.Wallets
	wallet, _ := w.CreateAppWallet(t.Context(), "treasury")
	pub, _ := wallet.Address.Bytes()
	unsigned := append([]byte{1}, make([]byte, 64)...)
	unsigned = append(append(append(unsigned, 1, 0, 0, 1), pub...), make([]byte, 33)...)
	signed, err := new(chainfake.Signer).SignTransaction(t.Context(), wallet.ID, unsigned)
	if tx, _ := chain.DecodeTransaction(signed); err != nil || !tx.Signed(0) {
		t.Fatalf("SignTransaction = %v", err)
	}
	if _, err := new(chainfake.Signer).SignTransaction(t.Context(), "other", unsigned); err == nil {
		t.Fatal("another wallet signed")
	}
}

func TestLedger_statusesFollowTheClock(t *testing.T) {
	t.Parallel()
	clk := testkit.NewClock(clock.Real{}.Now())
	l := chainfake.NewLedger(clk)
	l.SetSOL("relayer", 1_000_001)
	l.SetTokens("member", chain.Mint{Address: "usdc", Decimals: 6}, 5)
	sol, _ := l.SOLBalance(t.Context(), "relayer")
	usdc, _ := l.TokenBalance(t.Context(), "member", chain.Mint{Address: "usdc", Decimals: 6})
	if sol != money.NewBaseUnits(1_000_001, 9) || usdc != money.NewBaseUnits(5, 6) {
		t.Fatalf("balances %v %v", sol, usdc)
	}
	l.Land("sig", false)
	clk.Advance(time.Second)
	got, _ := l.SignatureStatuses(t.Context(), []chain.Signature{"sig", "gone"})
	if got[0].State != solana.StateProcessing || got[1].State != solana.StateNotFound || got[1].BlockHeight != 2 {
		t.Fatalf("statuses %+v", got)
	}
	clk.Advance(chainfake.Finality)
	got, _ = l.SignatureStatuses(t.Context(), []chain.Signature{"sig"})
	if got[0].State != solana.StateFinalized {
		t.Fatalf("after finality %+v", got)
	}
}
