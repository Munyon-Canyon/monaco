package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const (
	CashOutSweepInterval = 30 * time.Second
	CashOutSweepStale    = 30 * time.Second
	cashOutSweepBatch    = 50
)

func (c *CashOutPayouts) SweepDue(ctx context.Context) (int, error) {
	now := c.d.Clock.Now()
	due, err := sqlc.New(c.d.Reads).CashOutPayoutsDue(ctx, sqlc.CashOutPayoutsDueParams{
		StaleBefore: now.Add(-CashOutSweepStale),
		At:          now,
		MaxRows:     cashOutSweepBatch,
		RetryBaseSecs: int32(
			domain.PayoutRetryBase / time.Second,
		),
		RetryMaxSecs: int32(domain.PayoutRetryCeiling / time.Second),
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeInternal, "treasury.CashOutSweeper.list")
	}
	failures := make([]error, 0, len(due))
	for _, id := range due {
		failures = append(failures, c.rotate(ctx, id, now), c.Advance(ctx, id, 0, nil))
	}
	return len(due), errors.Join(failures...)
}

func (c *CashOutPayouts) rotate(ctx context.Context, id uuid.UUID, at time.Time) error {
	return c.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := sqlc.New(tx.Queries()).TouchCashOutJob(ctx, sqlc.TouchCashOutJobParams{ID: id, At: at})
		return err
	})
}
