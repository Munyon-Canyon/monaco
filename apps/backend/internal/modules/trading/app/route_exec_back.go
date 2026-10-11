package app

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (r *RouteExec) BridgeBack(ctx context.Context, id ids.SwapID) error {
	b, err := r.load(ctx, id.UUID())
	if err != nil {
		return err
	}
	landed, err := b.sent.Sub(b.fee)
	if err != nil {
		return err
	}
	leg := b.bridgeBack(landed)
	if err := r.d.Layer.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		err := q.LockStrandedCash(ctx, id.UUID())
		stranded := false
		if err == nil {
			stranded, err = q.StrandedCash(ctx, id.UUID())
		}
		if err != nil || !stranded {
			return err
		}
		_, err = r.d.Journal.begin(ctx, tx, leg.Ref, landed)
		return err
	}); err != nil {
		return err
	}
	if _, claimed, err := r.d.Journal.Step(ctx, id, domain.StepBridgeBack); err != nil || !claimed {
		return err
	}
	_, err = Bridge{Journal: r.d.Journal, Actions: r.d.Actions, Poll: r.d.Poll}.Run(ctx, leg)
	return err
}

func (b routedBuy) bridgeBack(landed money.Micros) BridgeLeg {
	return BridgeLeg{
		Ref: b.ref(domain.StepBridgeBack), WalletID: b.evm.ID, Fee: b.fee,
		Spec: multichain.TransferSpec{
			From: b.listing.Network, To: chain.NetworkSolana, Destination: string(b.solana), AmountMicros: landed,
		},
	}
}
