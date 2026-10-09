package app

import (
	"math"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDepositWatchRotationAndDiscoveryReportAnUnreadableDatabase(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	pool.Close()
	p := testWatch(pool, nil, &watchRPC{}, nil)
	if _, _, err := p.rotate(t.Context()); err == nil {
		t.Fatal("rotate error = nil")
	}
	if _, _, err := p.discover(t.Context()); err == nil {
		t.Fatal("discover error = nil")
	}
}

func TestDepositWatchRotationReportsScanFailures(t *testing.T) {
	t.Parallel()
	p := testWatch(nil, nil, &watchRPC{sigErr: errs.New(errs.CodeInternal, "test.sigs")}, nil)
	row := sqlc.DepositWatchRotationAccountsRow{TokenAccount: "account"}
	if _, err := p.recoverAccount(t.Context(), row); err == nil {
		t.Fatal("recoverAccount error = nil")
	}
	huge := []solana.SignatureInfo{{Signature: "huge", Slot: math.MaxInt64 + 1}}
	if _, err := p.commitRecoveryPage(t.Context(), sqlc.DepositWatchRotationAccountsRow{}, huge, false); err == nil {
		t.Fatal("commitRecoveryPage slot overflow error = nil")
	}
}

func TestDepositWatchRotationReturnsCommitFailures(t *testing.T) {
	t.Parallel()
	page := []solana.SignatureInfo{{Signature: "signature", Slot: 1}}
	for _, table := range []string{"deposit_candidates", "deposit_watch_accounts"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			p, ctx, user := watchWithDroppedTable(t, table)
			row := sqlc.DepositWatchRotationAccountsRow{
				TokenAccount: "account", WalletAddress: string(user.Address), UserID: user.ID.UUID(),
			}
			for _, more := range []bool{true, false} {
				if _, err := p.commitRecoveryPage(ctx, row, page, more); err == nil {
					t.Fatalf("commitRecoveryPage without %s (more=%t) error = nil", table, more)
				}
			}
		})
	}
}

func TestDepositWatchDiscoveryReportsFailures(t *testing.T) {
	t.Parallel()
	const wallet = "wallet"
	p := testWatch(nil, nil, &watchRPC{tokenErr: errs.New(errs.CodeInternal, "test.tokens")}, nil)
	if err := p.discoverWallet(t.Context(), wallet); errs.CodeOf(err) != errs.CodeRPCUnavailable {
		t.Fatalf("discoverWallet token accounts error = %v, want %s", err, errs.CodeRPCUnavailable)
	}
	p = testWatch(nil, nil, &watchRPC{slot: math.MaxInt64 + 1}, nil)
	if err := p.discoverWallet(t.Context(), wallet); err == nil {
		t.Fatal("discoverWallet slot overflow error = nil")
	}
	accounts := []solana.TokenAccountState{{Address: "account", Exists: true}}
	for _, table := range []string{"deposit_watch_accounts", "deposit_watch_wallets"} {
		t.Run(table, func(t *testing.T) {
			t.Parallel()
			p, ctx, _ := watchWithDroppedTable(t, table)
			p.rpc = &watchRPC{accounts: accounts}
			if err := p.discoverWallet(ctx, wallet); err == nil {
				t.Fatalf("discoverWallet without %s error = nil", table)
			}
		})
	}
}
