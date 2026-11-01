package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
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

func (c *CashOutPayouts) SweepSales(ctx context.Context) (int, error) {
	now := c.d.Clock.Now()
	stuck, err := sqlc.New(c.d.Reads).CashOutSalesStuck(ctx, sqlc.CashOutSalesStuckParams{
		StaleBefore: now.Add(-domain.CashOutSellStale), MaxRows: cashOutSweepBatch,
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeInternal, "treasury.CashOutSweeper.sales")
	}
	sales := NewCashOutSales(c.d.Ledger, c.d.IDs)
	failures := make([]error, 0, len(stuck))
	for _, row := range stuck {
		failures = append(failures, c.redrive(ctx, sales, row, now))
	}
	return len(stuck), errors.Join(failures...)
}

func (c *CashOutPayouts) redrive(
	ctx context.Context, sales CashOutSales, row sqlc.CashOutSalesStuckRow, now time.Time,
) error {
	cabal := ids.CabalIDFrom(row.CabalID)
	giveUp := row.CreatedAt.Before(now.Add(-domain.CashOutSellGiveUp))
	action := "reemit"
	if giveUp {
		action = "give_up"
	}
	claimed := false
	err := c.d.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		claimed = false
		lockErr := sales.ledger.LockCabal(ctx, tx, cabal)
		var n int64
		var claimErr error
		if lockErr == nil {
			n, claimErr = sqlc.New(tx.Queries()).ClaimStuckCashOutSale(ctx, sqlc.ClaimStuckCashOutSaleParams{
				ID: row.ID, At: now, StaleBefore: now.Add(-domain.CashOutSellStale),
			})
		}
		if err := errors.Join(lockErr, claimErr); err != nil || n == 0 {
			return err
		}
		claimed = true
		if giveUp {
			return sales.sell(ctx, tx, cabal, row.ID, true, now)
		}
		started, err := cashOutStartedFor(row)
		if err == nil {
			err = tx.Events.Append(ctx, started)
		}
		return err
	})
	if err != nil || !claimed {
		return err
	}
	observability.Alert(ctx, observability.TreasuryCashOutSaleStuck,
		slog.String("job_id", row.ID.String()), slog.String("cabal_id", cabal.String()),
		slog.String("action", action), slog.Int64("age_seconds", int64(now.Sub(row.CreatedAt)/time.Second)))
	return nil
}

func cashOutStartedFor(row sqlc.CashOutSalesStuckRow) (events.CashOutStarted, error) {
	units, unitsErr := money.ParseSharesUnits(row.ShareUnits)
	payout, payoutErr := money.ParseMicros(row.PayoutMicros)
	sell, sellErr := money.ParseMicros(row.SellUsdcMicros)
	if err := errors.Join(unitsErr, payoutErr, sellErr); err != nil {
		return events.CashOutStarted{}, errs.Wrap(err, errs.CodeDecodeFailed, "treasury.CashOutSweeper.started")
	}
	return events.CashOutStarted{
		V: 1, JobID: row.ID, CabalID: row.CabalID, UserID: row.UserID, ShareUnits: units.Uint64(),
		PayoutMicros: payout, SellUSDC: sell, Cause: row.Cause,
	}, nil
}
