package adapters

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const EVMDepositArrivalWindow = app.EVMDepositArrivalWindow

type EVMDepositArrival struct{}

func (EVMDepositArrival) Handle(ctx context.Context, tx db.Tx, e events.DepositCredited, at time.Time) error {
	arrived, err := sqlc.New(tx.Queries()).ArriveEVMDepositForward(ctx, sqlc.ArriveEVMDepositForwardParams{
		UserID: e.UserID, SolanaSignature: string(e.TxSignature), AmountMicros: e.AmountMicros.String(),
		CreatedAfter: at.Add(-EVMDepositArrivalWindow), ArrivedAt: at,
	})
	if err != nil || len(arrived) == 0 {
		return err
	}
	row := arrived[0]
	tx.AfterCommit(func(ctx context.Context) {
		observability.Info(ctx, observability.FundingEVMForwardMoved,
			slog.String("forward_id", row.ID.String()), slog.String("user_id", e.UserID.String()),
			slog.String("amount_micros", row.AmountMicros),
			slog.String("status_before", "pending"), slog.String("status_after", "arrived"))
	})
	return nil
}

func (h HTTP) arrivingMicros(ctx context.Context, user ids.UserID) *string {
	if h.Arriving == nil {
		return nil
	}
	arriving, err := h.Arriving.ArrivingMicros(ctx, user)
	if err != nil {
		observability.Degraded(ctx, observability.FundingEVMForwardArrivingUnavailable,
			slog.String("user_id", user.String()), slog.String("code", string(errs.CodeOf(err))))
		return nil
	}
	value := arriving.String()
	return &value
}
