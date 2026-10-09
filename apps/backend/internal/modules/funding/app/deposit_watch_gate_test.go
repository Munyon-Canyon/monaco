package app

import (
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/solana"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestDepositWatchGateReportsStorageFailures(t *testing.T) {
	t.Parallel()
	p, ctx, user := watchWithDroppedTable(t, "deposit_watch_accounts")
	if _, _, err := p.gate(ctx); err == nil {
		t.Fatal("gate error = nil without the account table")
	}
	p.rpc.(*watchRPC).fixed = []solana.TokenAccountState{{
		Address: "account", Exists: true, Program: chain.SPLProgram, Mint: testkit.USDCMint, Owner: user.Address,
		State: "initialized",
	}}
	row := sqlc.DepositWatchGateAccountsRow{TokenAccount: "account", WalletAddress: string(user.Address), State: "open"}
	if err := p.gatePage(ctx, []sqlc.DepositWatchGateAccountsRow{row}); err == nil {
		t.Fatal("gatePage error = nil without the account table")
	}
}
