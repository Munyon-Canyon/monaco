package app

import (
	"context"
	"errors"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
)

const (
	CashOutSweepInterval = 30 * time.Second
	CashOutSweepStale    = 2 * time.Minute
	cashOutSweepBatch    = 50
)

func (c *CashOutPayouts) SweepDue(ctx context.Context) (int, error) {
	due, err := sqlc.New(c.d.Reads).CashOutPayoutsDue(ctx, sqlc.CashOutPayoutsDueParams{
		StaleBefore: c.d.Clock.Now().Add(-CashOutSweepStale), MaxRows: cashOutSweepBatch,
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeInternal, "treasury.CashOutSweeper.list")
	}
	failures := make([]error, 0, len(due))
	for _, id := range due {
		failures = append(failures, c.Advance(ctx, id, 0, nil))
	}
	return len(due), errors.Join(failures...)
}
