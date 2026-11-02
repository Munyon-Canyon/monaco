package app

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

func (r *RouteExec) sold(ctx context.Context, b routedBuy, fill EVMSwapResult) error {
	proceeds, err := fill.Out.Micros()
	if err != nil {
		return err
	}
	err = r.finish(ctx, b, b.sold(fill, proceeds), "", func(ctx context.Context, tx db.Tx) error {
		_, err := r.d.Journal.begin(ctx, tx, b.ref(domain.StepBridgeBack), proceeds)
		return err
	})
	if err != nil {
		return err
	}
	return r.BridgeBack(ctx, ids.SwapIDFrom(b.id))
}

func (b routedBuy) sellLeg() EVMSwapLeg {
	return EVMSwapLeg{
		Request: b.req, Ref: b.ref(domain.StepSwap), WalletID: b.evm.ID,
		Amount: b.sent,
		Spec: EVMSwapSpec{
			Network: b.listing.Network, Owner: b.evm.Address, TokenIn: b.token, TokenOut: b.cash,
			AmountIn: b.req.InAmount, SlippageBps: b.slippage,
		},
	}
}

func (b routedBuy) sold(fill EVMSwapResult, proceeds money.Micros) step {
	req, id := b.req, b.id
	return step{
		id: id, from: domain.StatusSubmitted, status: domain.StatusConfirmed,
		apply: func(ctx context.Context, q *sqlc.Queries, at time.Time) (int64, error) {
			return q.FinishConfirmed(
				ctx,
				sqlc.FinishConfirmedParams{ID: id, OutAmount: fill.Out.String(), ConfirmedAt: at},
			)
		},
		event: func(at time.Time) events.Event {
			return events.TradeConfirmed{
				V: 1, SwapID: id, CabalID: req.CabalID.UUID(), Source: req.tradeSource(),
				SourceBatchSize: req.SourceBatchSize, Action: string(req.Action), Symbol: req.Symbol,
				InMint: req.InMint.Address, InAmount: req.InAmount, OutMint: req.OutMint.Address,
				OutAmount: fill.Out, USDCMicros: proceeds,
				TxSignature: chain.Signature(fill.Hash), ConfirmedAt: at, TokenDecimals: b.listing.Decimals,
			}
		},
	}
}
