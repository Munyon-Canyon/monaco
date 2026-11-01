package adapters

import (
	"context"
	"log/slog"
	"math/big"
	"strings"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	tradeStepsOp = "treasury.TradeSteps.Handle"
	reusedPrefix = "reused:"
)

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
		err = landed(ctx, tx, q, ledger, b, e)
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
	net, err := b.Sent.Sub(b.Fee)
	if err != nil {
		net = money.Micros{}
	}
	if net.IsZero() {
		return nil
	}
	return q.InsertInTransit(ctx, sqlc.InsertInTransitParams{
		CabalID: b.CabalID.UUID(), TradeID: b.SwapID, Network: string(b.Network), CreatedAt: at,
		AmountMicros: pgtype.Numeric{Int: new(big.Int).SetUint64(net.Uint64()), Valid: true},
	})
}

func landed(
	ctx context.Context, tx db.Tx, q *sqlc.Queries, ledger app.Ledger, b domain.Bridge, e events.TradeStepChanged,
) error {
	var err error
	if b.SwapID, err = bookedOn(e); err != nil {
		return err
	}
	err = finish(ctx, q, b, func() error { return ledger.BridgeLanded(ctx, tx, b) })
	if err != nil || b.Direction != domain.BridgeBack {
		return err
	}
	return q.RecordSellBridgeFee(ctx, sqlc.RecordSellBridgeFeeParams{
		TradeID: e.SwapID, FeeMicros: e.FeeMicros.String(),
	})
}

func bookedOn(e events.TradeStepChanged) (uuid.UUID, error) {
	from, reused := strings.CutPrefix(e.PrivyActionID, reusedPrefix)
	if !reused {
		return e.SwapID, nil
	}
	id, err := uuid.Parse(from)
	if err != nil {
		return uuid.UUID{}, errs.Wrap(err, errs.CodeInvalidInput, tradeStepsOp,
			slog.String("privy_action_id", e.PrivyActionID))
	}
	return id, nil
}

func finish(ctx context.Context, q *sqlc.Queries, b domain.Bridge, post func() error) error {
	_, err := q.DeleteInTransit(ctx, sqlc.DeleteInTransitParams{TradeID: b.SwapID, Network: string(b.Network)})
	if err != nil {
		return err
	}
	return post()
}
