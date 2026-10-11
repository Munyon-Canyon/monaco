package app

import (
	"context"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const (
	bridgeSubmitted = "submitted"
	bridgeSucceeded = "succeeded"
	bridgeFailed    = "failed"
)

func (l Ledger) BridgeSent(ctx context.Context, tx db.Tx, b domain.Bridge) (bool, error) {
	id := b.TxnID(bridgeSubmitted)
	if err := l.LockCabal(ctx, tx, b.CabalID); err != nil {
		return false, err
	}
	rows, err := sqlc.New(tx.Queries()).AdminTxnByID(ctx, id)
	if err != nil || len(rows) > 0 {
		return false, err
	}
	txn, err := domain.NewBridgeSentTxn(id, b, l.usdc)
	if err != nil {
		return false, err
	}
	return true, l.PostCabalTxn(ctx, tx, txn)
}

func (l Ledger) BridgeLanded(ctx context.Context, tx db.Tx, b domain.Bridge) error {
	return l.finishBridge(ctx, tx, b, bridgeSucceeded, domain.NewBridgeLandedTxn)
}

func (l Ledger) BridgeReturned(ctx context.Context, tx db.Tx, b domain.Bridge) error {
	return l.finishBridge(ctx, tx, b, bridgeFailed, domain.NewBridgeReturnedTxn)
}

func (l Ledger) finishBridge(
	ctx context.Context, tx db.Tx, b domain.Bridge, status string,
	build func(id uuid.UUID, b domain.Bridge, usdc domain.Asset) (domain.CabalTxn, error),
) error {
	if _, err := l.BridgeSent(ctx, tx, b); err != nil {
		return err
	}
	txn, err := build(b.TxnID(status), b, l.usdc)
	if err != nil {
		return err
	}
	return l.PostCabalTxn(ctx, tx, txn)
}
