package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type DepositRescan struct {
	UoW *db.UnitOfWork
}

func (r DepositRescan) Rescan(ctx context.Context, wallet chain.SolanaAddress, from chain.Signature) (int, error) {
	const op = "funding.DepositRescan.Rescan"
	if wallet == "" {
		return 0, errs.New(errs.CodeInvalidInput, op, slog.String("reason", "wallet is empty"))
	}
	var accounts int64
	err := r.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		accounts, err = sqlc.New(tx.Queries()).RescanDepositWatchWallet(ctx, sqlc.RescanDepositWatchWalletParams{
			WalletAddress: string(wallet), FromSignature: string(from),
		})
		return err
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), op)
	}
	if accounts == 0 {
		return 0, errs.New(errs.CodeNotFound, op, slog.String("wallet_address", string(wallet)))
	}
	observability.Info(ctx, observability.FundingDepositRescanned,
		slog.String("wallet_address", string(wallet)), slog.String("from_signature", string(from)),
		slog.Int("accounts", int(accounts)))
	return int(accounts), nil
}
