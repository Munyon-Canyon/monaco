package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

func (l Ledger) WriteOffEVMCash(ctx context.Context, tx db.Tx, w domain.CashWriteOff) error {
	if err := l.LockCabal(ctx, tx, w.CabalID); err != nil {
		return err
	}
	if posted, err := txnPosted(ctx, tx, w.TxnID()); err != nil || posted {
		return err
	}
	txn, err := domain.NewCashWriteOffTxn(w)
	if err != nil {
		return err
	}
	return l.PostCabalTxn(ctx, tx, txn)
}
