package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const cursorMigrationPage = 500

type CursorMigrator struct {
	reads sqlc.DBTX
	uow   *db.UnitOfWork
	clock clock.Clock
	usdc  chain.SolanaAddress
	page  int32
}

func NewCursorMigrator(reads sqlc.DBTX, uow *db.UnitOfWork, c clock.Clock, usdc chain.SolanaAddress) CursorMigrator {
	return CursorMigrator{reads: reads, uow: uow, clock: c, usdc: usdc, page: cursorMigrationPage}
}

func (m CursorMigrator) Run(ctx context.Context) (int, error) {
	added, after := 0, ""
	for {
		rows, err := sqlc.New(m.reads).DepositCursorsToMigrate(ctx, sqlc.DepositCursorsToMigrateParams{
			WalletAddress: after, Limit: m.page,
		})
		if err != nil {
			return added, errs.Wrap(err, errs.CodeOf(err), "funding.CursorMigrator.read")
		}
		n, err := m.migrate(ctx, rows)
		added += n
		if err != nil || len(rows) < int(m.page) {
			return added, err
		}
		after = rows[len(rows)-1].WalletAddress
	}
}

func (m CursorMigrator) migrate(ctx context.Context, rows []sqlc.DepositCursorsToMigrateRow) (int, error) {
	added := 0
	err := m.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		for _, row := range rows {
			ata, err := chain.AssociatedTokenAccount(chain.SolanaAddress(row.WalletAddress), m.usdc, chain.SPLProgram)
			if err != nil {
				return errs.Wrap(err, errs.CodeInvalidAddress, "funding.CursorMigrator.ata")
			}
			n, err := q.InsertDepositWatchWallet(ctx, sqlc.InsertDepositWatchWalletParams{
				WalletAddress: row.WalletAddress, UserID: row.UserID, FirstSeenSlot: row.CursorSlot,
				FirstSeenAt: m.clock.Now(),
			})
			if err != nil {
				return err
			}
			added += int(n)
			if err := q.InsertDepositWatchAccount(ctx, sqlc.InsertDepositWatchAccountParams{
				TokenAccount: string(ata), WalletAddress: row.WalletAddress, Canonical: true, State: "open",
				LastAmount: "0", DirtyGen: 1, DirtySlot: row.CursorSlot, HighSignature: row.LastSignature,
				HighSlot: row.CursorSlot, RecoveryDueAt: m.clock.Now(),
			}); err != nil {
				return err
			}
		}
		return nil
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "funding.CursorMigrator.migrate")
	}
	return added, nil
}
