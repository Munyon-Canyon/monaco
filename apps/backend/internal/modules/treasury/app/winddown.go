package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	WindDownInterval    = 2 * time.Minute
	WindDownMaxAttempts = 10
	WindDownMaxAge      = 24 * time.Hour
)

type WindDown struct {
	uow     *db.UnitOfWork
	cashOut *CashOutHandler
	reads   sqlc.DBTX
	clock   clock.Clock
}

func NewWindDown(uow *db.UnitOfWork, cashOut *CashOutHandler, reads sqlc.DBTX, c clock.Clock) *WindDown {
	return &WindDown{uow: uow, cashOut: cashOut, reads: reads, clock: c}
}

func windDownCashOut(cabal ids.CabalID, user ids.UserID) CashOut {
	return CashOut{CabalID: cabal, UserID: user, All: true, Cause: domain.CashOutWindDown}
}

func deferred(err error) bool {
	code := errs.CodeOf(err)
	return code == errs.CodeCashOutInProgress || code == errs.CodePotValueChanged ||
		code == errs.CodePriceUnavailable || code == errs.CodeInsufficientShares || code == errs.CodeCabalPaused
}

func (w *WindDown) Start(ctx context.Context, tx db.Tx, e events.CabalBanned, at time.Time) error {
	q := sqlc.New(tx.Queries())
	started, err := q.StartWindDown(ctx, sqlc.StartWindDownParams{CabalID: e.CabalID, At: at})
	if err != nil || started == 0 {
		return err
	}
	cabal := ids.CabalIDFrom(e.CabalID)
	holders, err := q.WindDownHolders(ctx, e.CabalID)
	if err != nil {
		return err
	}
	issued := 0
	for _, holder := range holders {
		_, err := w.cashOut.Handle(ctx, windDownCashOut(cabal, ids.UserIDFrom(holder)))
		if err == nil {
			issued++
		} else if !deferred(err) {
			observability.Debug(
				ctx,
				observability.TreasuryWindDownMemberFailed,
				slog.String("cabal_id", cabal.String()),
				slog.String("user_id", holder.String()),
				slog.String("code", string(errs.CodeOf(err))),
			)
		}
	}
	faultpoint.Hit(ctx, faultpoint.BeforeCommit)
	tx.AfterCommit(func(ctx context.Context) {
		observability.Info(ctx, observability.TreasuryWindDownStarted, slog.String("cabal_id", cabal.String()),
			slog.Int("holders", len(holders)), slog.Int("started", issued))
	})
	return nil
}

func (w *WindDown) Advance(ctx context.Context) (scanned, changed int, err error) {
	rows, err := sqlc.New(w.reads).RunningWindDowns(ctx)
	if err != nil {
		return 0, 0, err
	}
	var failed error
	for _, row := range rows {
		done, err := w.advance(ctx, ids.CabalIDFrom(row.CabalID), int(row.Attempts), row.StartedAt)
		if done {
			changed++
		}
		if err != nil && failed == nil {
			failed = err
		}
	}
	return len(rows), changed, failed
}

func (w *WindDown) advance(ctx context.Context, cabal ids.CabalID, attempts int, startedAt time.Time) (bool, error) {
	holders, err := sqlc.New(w.reads).WindDownHolders(ctx, cabal.UUID())
	if err != nil {
		return false, err
	}
	if len(holders) == 0 {
		return w.complete(ctx, cabal)
	}
	stuck := func() {
		observability.Alert(ctx, observability.TreasuryWindDownStuck, slog.String("cabal_id", cabal.String()),
			slog.Int("attempts", attempts), slog.Int("holders", len(holders)))
	}
	if attempts >= WindDownMaxAttempts {
		stuck()
		return false, errs.New(errs.CodeInternal, "treasury.WindDown.advance", slog.String("cabal_id", cabal.String()))
	}
	if w.clock.Now().Sub(startedAt) > WindDownMaxAge {
		stuck()
	}
	issued := 0
	for _, holder := range holders {
		_, err := w.cashOut.Handle(ctx, windDownCashOut(cabal, ids.UserIDFrom(holder)))
		if err == nil {
			issued++
		} else if !deferred(err) {
			return false, err
		}
	}
	if issued == 0 {
		return false, nil
	}
	err = w.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := sqlc.New(tx.Queries()).BumpWindDownAttempts(ctx, cabal.UUID())
		return err
	})
	return err == nil, err
}

func (w *WindDown) complete(ctx context.Context, cabal ids.CabalID) (bool, error) {
	var totals sqlc.CompleteWindDownRow
	done := false
	err := w.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		totals, err = sqlc.New(tx.Queries()).CompleteWindDown(ctx, sqlc.CompleteWindDownParams{
			CabalID: cabal.UUID(), At: w.clock.Now(),
		})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		if err != nil {
			return err
		}
		done = true
		faultpoint.Hit(ctx, faultpoint.BeforeCommit)
		return tx.Events.Append(ctx, events.CabalWoundDown{
			V: 1, CabalID: cabal.UUID(), MembersPaid: totals.MembersPaid,
			USDCReturnedMicros: money.MicrosFromUint64(uint64(max(totals.ReturnedMicros, 0))),
		})
	})
	if done && err == nil {
		observability.Info(ctx, observability.TreasuryWindDownCompleted, slog.String("cabal_id", cabal.String()),
			slog.Int64("members_paid", totals.MembersPaid), slog.Int64("returned_micros", totals.ReturnedMicros))
	}
	return done && err == nil, err
}
