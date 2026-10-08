package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type Triggers struct{}

func (Triggers) Traded(ctx context.Context, tx db.Tx, e events.TradeConfirmed, at time.Time) error {
	return trigger(ctx, tx, e.CabalID, "trade_confirmed", at)
}

func (Triggers) Funded(ctx context.Context, tx db.Tx, e events.Funded, at time.Time) error {
	return trigger(ctx, tx, e.CabalID, "cabal_funded", at)
}

func (Triggers) CashedOut(ctx context.Context, tx db.Tx, e events.CashOutCompleted, at time.Time) error {
	return trigger(ctx, tx, e.CabalID, "cashout_completed", at)
}

func (Triggers) CashOutStarted(ctx context.Context, tx db.Tx, e events.CashOutStarted, at time.Time) error {
	return trigger(ctx, tx, e.CabalID, "cashout_started", at)
}

func (Triggers) CashOutPartial(ctx context.Context, tx db.Tx, e events.CashOutPartial, at time.Time) error {
	return trigger(ctx, tx, e.CabalID, "cashout_partial", at)
}

func (Triggers) CashOutFailed(ctx context.Context, tx db.Tx, e events.CashOutFailed, at time.Time) error {
	return trigger(ctx, tx, e.CabalID, "cashout_failed", at)
}
