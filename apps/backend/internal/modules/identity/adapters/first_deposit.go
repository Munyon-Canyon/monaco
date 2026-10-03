package adapters

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type FirstDeposit struct{}

func (FirstDeposit) Handle(ctx context.Context, tx db.Tx, e events.DepositCredited, at time.Time) error {
	if e.AmountMicros.Cmp(domain.FirstDepositThreshold()) < 0 {
		observability.Info(ctx, observability.IdentityFirstDepositBelowThreshold,
			slog.String("amount_micros", e.AmountMicros.String()),
			slog.String("threshold_micros", domain.FirstDepositThreshold().String()))
		return nil
	}
	n, err := sqlc.New(tx.Queries()).SetFirstDepositAt(ctx, sqlc.SetFirstDepositAtParams{
		DepositedAt: blockTimeOrDelivery(e, at), Now: at, ID: e.UserID,
	})
	if err != nil {
		return err
	}
	if n == 0 {
		observability.Info(ctx, observability.IdentityFirstDepositAlreadySet, slog.String("user_id", e.UserID.String()))
	}
	return nil
}

func blockTimeOrDelivery(e events.DepositCredited, deliveredAt time.Time) time.Time {
	if e.BlockTime == nil {
		return deliveredAt
	}
	return *e.BlockTime
}
