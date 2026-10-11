package adapters

import (
	"context"
	"log/slog"
	"math/big"
	"time"

	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const tradeStepsOp = "treasury.TradeSteps.Handle"

type TradeSteps struct {
	Ledger app.Ledger
	Hints  Hints
}

func (h TradeSteps) Handle(ctx context.Context, tx db.Tx, e events.TradeStepChanged, at time.Time) error {
	direction := domain.BridgeDirection(e.Step)
	if direction != domain.BridgeOut && direction != domain.BridgeBack {
		return nil
	}
	if !e.Network.IsEVM() {
		return errs.New(errs.CodeInvalidInput, tradeStepsOp, slog.String("network", string(e.Network)))
	}
	b := domain.Bridge{
		SwapID: e.SwapID, CabalID: ids.CabalIDFrom(e.CabalID), Direction: direction, Network: e.Network,
		Sent: e.AmountMicros, Fee: e.FeeMicros, Attempt: e.Attempt,
	}
	ledger := h.Ledger.At(at)
	q := sqlc.New(tx.Queries())
	var err error
	switch e.Status {
	case "pending":
		return nil
	case "submitted":
		err = h.sent(ctx, tx, q, ledger, b, at)
	case "succeeded":
		err = finish(ctx, q, b, func() error { return ledger.BridgeLanded(ctx, tx, b) })
	case "failed":
		if e.PrivyActionID == "" {
			return nil
		}
		err = finish(ctx, q, b, func() error { return ledger.BridgeReturned(ctx, tx, b) })
	default:
		return errs.New(errs.CodeInvalidInput, tradeStepsOp, slog.String("status", e.Status))
	}
	if err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) {
		h.Hints.PublishHint(ctx, events.CabalActivityChangedHint(b.CabalID), nil)
	})
	return nil
}

func (TradeSteps) sent(
	ctx context.Context, tx db.Tx, q *sqlc.Queries, ledger app.Ledger, b domain.Bridge, at time.Time,
) error {
	posted, err := ledger.BridgeSent(ctx, tx, b)
	if err != nil || !posted {
		return err
	}
	return q.InsertInTransit(ctx, sqlc.InsertInTransitParams{
		CabalID: b.CabalID.UUID(), TradeID: b.SwapID, Network: string(b.Network), CreatedAt: at,
		AmountMicros: pgtype.Numeric{Int: new(big.Int).SetUint64(b.Sent.Uint64()), Valid: true},
	})
}

func finish(ctx context.Context, q *sqlc.Queries, b domain.Bridge, post func() error) error {
	_, err := q.DeleteInTransit(ctx, sqlc.DeleteInTransitParams{TradeID: b.SwapID, Network: string(b.Network)})
	if err != nil {
		return err
	}
	return post()
}
