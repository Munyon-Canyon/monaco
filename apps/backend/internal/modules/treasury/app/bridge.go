package app

import (
	"context"
	"errors"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	bridgeSubmitted = "submitted"
	bridgeSucceeded = "succeeded"
	bridgeFailed    = "failed"
	bridgeAdjusted  = "adjust"
)

func (l Ledger) BridgeSent(ctx context.Context, tx db.Tx, b domain.Bridge) (bool, error) {
	id := b.TxnID(bridgeSubmitted)
	if err := l.LockCabal(ctx, tx, b.CabalID); err != nil {
		return false, err
	}
	if posted, err := txnPosted(ctx, tx, id); err != nil || posted {
		return false, err
	}
	txn, err := domain.NewBridgeSentTxn(id, b, l.usdc)
	if err != nil {
		return false, err
	}
	return true, l.PostCabalTxn(ctx, tx, txn)
}

func (l Ledger) BridgeLanded(ctx context.Context, tx db.Tx, b domain.Bridge) error {
	if err := l.finishBridge(ctx, tx, b, bridgeSucceeded, domain.NewBridgeLandedTxn); err != nil {
		return err
	}
	if b.Direction != domain.BridgeOut {
		return nil
	}
	return l.adjustLanded(ctx, tx, b)
}

func (l Ledger) adjustLanded(ctx context.Context, tx db.Tx, b domain.Bridge) error {
	id := b.TxnID(bridgeAdjusted)
	posted, err := txnPosted(ctx, tx, id)
	var booked money.Micros
	if err == nil && !posted {
		booked, err = bookedFee(ctx, tx, b.TxnID(bridgeSucceeded), domain.EVMCashAsset(b.Network))
	}
	if err != nil || posted {
		return err
	}
	txn, adjust, err := domain.NewBridgeAdjustedTxn(id, b, booked, l.usdc)
	if err != nil || !adjust {
		return err
	}
	return l.PostCabalTxn(ctx, tx, txn)
}

func bookedFee(ctx context.Context, tx db.Tx, landed uuid.UUID, asset domain.Asset) (money.Micros, error) {
	rows, err := sqlc.New(tx.Queries()).AdminTxnByID(ctx, landed)
	var fee money.Micros
	for _, r := range rows {
		if r.Account != string(domain.CabalFees) || r.Asset != string(asset) || r.Amount <= 0 {
			continue
		}
		credit, creditErr := money.SignedMicrosFromInt64(r.Amount).Micros()
		sum, sumErr := fee.Add(credit)
		fee, err = sum, errors.Join(err, creditErr, sumErr)
	}
	return fee, err
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
	id := b.TxnID(status)
	if posted, err := txnPosted(ctx, tx, id); err != nil || posted {
		return err
	}
	txn, err := build(id, b, l.usdc)
	if err != nil {
		return err
	}
	return l.PostCabalTxn(ctx, tx, txn)
}

func txnPosted(ctx context.Context, tx db.Tx, id uuid.UUID) (bool, error) {
	rows, err := sqlc.New(tx.Queries()).AdminTxnByID(ctx, id)
	return len(rows) > 0, err
}
