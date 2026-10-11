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
	amount, err := r.returning(ctx, b)
	if err != nil || amount.IsZero() {
		return err
	}
	_, err = r.bridge().Run(ctx, b.bridgeBack(amount))
	return err
}

func (r *RouteExec) returning(ctx context.Context, b routedBuy) (money.Micros, error) {
	step, started, err := r.d.Journal.Step(ctx, ids.SwapIDFrom(b.id), domain.StepBridgeBack)
	if err != nil || started {
		return step.Amount, err
	}
	landed, err := b.sent.Sub(b.fee)
	if err != nil {
		return money.Micros{}, err
	}
	claimed := false
	err = r.d.Layer.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		err := q.LockStrandedCash(ctx, b.id)
		if err == nil {
			claimed, err = q.StrandedCash(ctx, b.id)
		}
		if err != nil || !claimed {
			return err
		}
		_, err = r.d.Journal.begin(ctx, tx, b.ref(domain.StepBridgeBack), landed)
		return err
	})
	if err != nil || !claimed {
		return money.Micros{}, err
	}
	return landed, nil
}

func (b routedBuy) bridgeBack(landed money.Micros) BridgeLeg {
	return BridgeLeg{
		Ref: b.ref(domain.StepBridgeBack), WalletID: b.evm.ID, Fee: b.fee,
		Spec: multichain.TransferSpec{
			From: b.listing.Network, To: chain.NetworkSolana, Destination: string(b.solana), AmountMicros: landed,
		},
	}
}
