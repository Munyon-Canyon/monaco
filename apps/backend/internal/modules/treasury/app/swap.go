package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

func (l Ledger) PostSwap(ctx context.Context, tx db.Tx, s domain.Swap) error {
	txn, err := domain.NewSwapTxn(uuid.NewSHA1(s.ID, []byte("cabal_txn")), s, l.usdc)
	if err != nil {
		return err
	}
	if err := l.LockCabal(ctx, tx, s.CabalID); err != nil {
		return err
	}
	posted, err := sqlc.New(tx.Queries()).SwapPosted(ctx, s.ID)
	if err != nil || posted {
		return err
	}
	return l.PostCabalTxn(ctx, tx, txn)
}
