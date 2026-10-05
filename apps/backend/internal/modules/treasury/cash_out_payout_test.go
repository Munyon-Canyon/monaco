package treasury_test

import (
	"context"
	"testing"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	cabalport "github.com/monaco/monaco/apps/backend/internal/modules/cabal/port"
	identityport "github.com/monaco/monaco/apps/backend/internal/modules/identity/port"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/adapters"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	walletTreasury = chain.SolanaAddress("9xQeWvG816bUx9EPjHmaT23yvVMvM9fQj4a8PHF4H6P")
	walletMember   = chain.SolanaAddress("5kwEmpcR8Txq1b4bDazRm9j4cx8Qo2aiE53rYA1dCDDP")
)

type statuses struct {
	out []solana.Status
	err error
}

func (s statuses) SignatureStatuses(context.Context, []chain.Signature) ([]solana.Status, error) {
	return s.out, s.err
}

func TestPayoutChain_readsOneSignature(t *testing.T) {
	t.Parallel()
	for _, c := range []struct {
		name string
		in   statuses
		want domain.PayoutReading
		code errs.Code
	}{
		{
			"finalized failed",
			statuses{out: []solana.Status{{State: solana.StateFinalized, Failed: true}}},
			domain.PayoutReading{State: domain.PayoutFinalized, Failed: true},
			"",
		},
		{
			"processing",
			statuses{out: []solana.Status{{State: solana.StateProcessing, BlockHeight: 900}}},
			domain.PayoutReading{State: domain.PayoutProcessing},
			"",
		},
		{
			"not found, blockhash still valid",
			statuses{out: []solana.Status{{State: solana.StateNotFound, BlockHeight: 100}}},
			domain.PayoutReading{State: domain.PayoutNotFound},
			"",
		},
		{
			"not found past the blockhash",
			statuses{out: []solana.Status{{State: solana.StateNotFound, BlockHeight: 101}}},
			domain.PayoutReading{State: domain.PayoutNotFound, Expired: true},
			"",
		},
		{"rpc down", statuses{err: errs.New(errs.CodeRPCUnavailable, "test")}, domain.PayoutReading{}, errs.CodeRPCUnavailable},
		{"no answer", statuses{}, domain.PayoutReading{}, errs.CodeDecodeFailed},
		{"unknown state", statuses{out: []solana.Status{{State: 9}}}, domain.PayoutReading{}, errs.CodeDecodeFailed},
	} {
		got, err := adapters.PayoutChain{
			Solana: func() adapters.SignatureStatuses { return c.in },
		}.PayoutStatus(
			t.Context(),
			"sig",
			100,
		)
		if errs.CodeOf(err) != c.code && (c.code != "" || err != nil) || got != c.want {
			t.Fatalf("%s: PayoutStatus = %+v, %v", c.name, got, err)
		}
	}
}

func TestLedger_failUnpairedFailsOnlyAUserSideWithNoCabalSide(t *testing.T) {
	t.Parallel()
	f := newFixture(t)
	user, cabal := f.user(t), f.cabal(t)
	cashOutFundParts(t, f, user, cabal)
	lone, _, err := f.cashOut(user, cabal, 1_000_000, 1, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	pairU, pairC, err := f.cashOut(user, cabal, 1_000_000, 1, domain.TxnPending)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.do(
		func(ctx context.Context, tx db.Tx) error { return f.ledger.PostUserTxn(ctx, tx, lone) },
	); err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(pairU, pairC); err != nil {
		t.Fatal(err)
	}
	for _, c := range []struct {
		transfer uuid.UUID
		want     bool
	}{{lone.TransferID, true}, {lone.TransferID, false}, {pairU.TransferID, false}} {
		var failed bool
		if err := f.do(func(ctx context.Context, tx db.Tx) error {
			var err error
			failed, err = f.ledger.FailUnpaired(ctx, tx, c.transfer)
			return err
		}); err != nil || failed != c.want {
			t.Fatalf("FailUnpaired(%s) = %t, %v, want %t", c.transfer, failed, err, c.want)
		}
	}
}

func cashOutFundParts(t *testing.T, f fixture, user ids.UserID, cabal ids.CabalID) {
	t.Helper()
	u, c, err := f.fund(user, cabal, 10_000_000, 10, domain.TxnSettled)
	if err != nil {
		t.Fatal(err)
	}
	if err := f.postPair(u, c); err != nil {
		t.Fatal(err)
	}
}

func TestPayoutWallets_adaptCabalAndIdentityReads(t *testing.T) {
	t.Parallel()
	w := app.PayoutWalletReads{Cabals: walletReads{}, Members: walletReads{}}
	wallet, err := w.TreasuryWallet(t.Context(), ids.CabalID{})
	if err != nil || wallet != (chain.Wallet{ID: "treasury-wallet", Address: walletTreasury}) {
		t.Fatalf("TreasuryWallet = %+v, %v", wallet, err)
	}
	if addr, err := w.MemberAddress(t.Context(), ids.UserID{}); err != nil || addr != walletMember {
		t.Fatalf("MemberAddress = %s, %v", addr, err)
	}
	unwired := app.PayoutWalletReads{Cabals: app.UnwiredReads{}, Members: app.UnwiredReads{}}
	if _, err := unwired.TreasuryWallet(t.Context(), ids.CabalID{}); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("unwired TreasuryWallet = %v", err)
	}
}

type walletReads struct{}

func (walletReads) TreasuryWallet(context.Context, ids.CabalID) (cabalport.TreasuryWallet, error) {
	return cabalport.TreasuryWallet{PrivyWalletID: "treasury-wallet", Address: walletTreasury}, nil
}

func (walletReads) MemberWallet(context.Context, ids.UserID) (identityport.MemberWallet, error) {
	return identityport.MemberWallet{Address: walletMember}, nil
}

func TestUnwiredReads_payoutWalletsFailClosed(t *testing.T) {
	t.Parallel()
	if _, err := (app.UnwiredReads{}).TreasuryWallet(t.Context(), ids.CabalID{}); errs.CodeOf(err) !=
		errs.CodeUpstreamUnavailable {
		t.Fatalf("TreasuryWallet = %v", err)
	}
	if _, err := (app.UnwiredReads{}).MemberWallet(t.Context(), ids.UserID{}); errs.CodeOf(err) !=
		errs.CodeUpstreamUnavailable {
		t.Fatalf("MemberWallet = %v", err)
	}
}
