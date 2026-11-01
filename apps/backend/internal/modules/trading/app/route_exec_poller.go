package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"strconv"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	StrandedReturnHold    = 24 * time.Hour
	MaxBridgeBackAttempts = 5
	bridgeBackSweepActor  = "trading.bridge_back_sweep"
)

type Resumer interface {
	Resume(ctx context.Context, id ids.SwapID) error
	BridgeBack(ctx context.Context, id ids.SwapID) error
}

type EVMTrades struct {
	uow       *db.UnitOfWork
	reads     sqlc.DBTX
	clock     clock.Clock
	routes    Resumer
	proposals Proposals
	timing    SweepTiming
	retry     *RetryBridgeBackHandler
}

func NewEVMTrades(
	uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, routes Resumer, proposals Proposals, timing SweepTiming,
) *EVMTrades {
	return &EVMTrades{
		uow: uow, reads: reads, clock: c, routes: routes, proposals: proposals, timing: timing,
		retry: NewRetryBridgeBackHandler(uow, reads, c, ids.Real{}),
	}
}

func (*EVMTrades) Name() string { return "trading.evm_trades" }

func (p *EVMTrades) Interval() time.Duration { return p.timing.Interval }

func (p *EVMTrades) Tick(ctx context.Context) (poller.Report, error) {
	now := p.clock.Now()
	q := sqlc.New(p.reads)
	unfinished, unfinishedErr := q.ListRoutedUnfinished(ctx, sqlc.ListRoutedUnfinishedParams{
		OlderThan: now.Add(-p.timing.Age), Now: now, MaxRows: SwapSweepBatch,
	})
	stranded, strandedErr := q.ListStrandedCash(ctx, sqlc.ListStrandedCashParams{
		OlderThan: now.Add(-p.timing.Age), MaxRows: SwapSweepBatch,
	})
	failed, failedErr := p.retry.ListFailed(ctx)
	if err := errors.Join(unfinishedErr, strandedErr, failedErr); err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeInternal, "trading.EVMTrades")
	}
	report := poller.Report{Scanned: len(unfinished) + len(stranded)}
	failures := make([]error, 0, len(unfinished)+len(stranded))
	for _, id := range unfinished {
		_, err := q.TouchRouted(ctx, sqlc.TouchRoutedParams{ID: id, At: now})
		if err == nil {
			err = p.resume(ctx, id, now)
		}
		if err == nil {
			report.Changed++
		}
		failures = append(failures, err)
	}
	for _, row := range stranded {
		returned, err := p.bridgeBack(ctx, q, row, now)
		if returned && err == nil {
			report.Changed++
		}
		failures = append(failures, err)
	}
	failures = append(failures, p.retryFailedBridgeBacks(ctx, failed, now, &report))
	return report, errors.Join(failures...)
}

func (p *EVMTrades) retryFailedBridgeBacks(
	ctx context.Context, failed []FailedBridgeBack, now time.Time, report *poller.Report,
) error {
	var failures []error
	for _, row := range failed {
		if now.Sub(row.FailedAt) < p.timing.Age {
			continue
		}
		report.Scanned++
		retrying := row.Attempt < MaxBridgeBackAttempts
		observability.Alert(ctx, observability.TradingBridgeBackFailed,
			slog.String("swap_id", row.SwapID.String()), slog.String("cabal_id", row.CabalID.String()),
			slog.Int("attempt", row.Attempt), slog.String("error", row.Error), slog.Bool("retrying", retrying))
		if !retrying {
			continue
		}
		_, err := p.retry.Handle(ctx, RetryBridgeBack{
			SwapID: row.SwapID,
			Actor:  bridgeBackSweepActor,
			Reason: "the return failed on attempt " + strconv.Itoa(row.Attempt),
		})
		if err == nil {
			report.Changed++
		}
		failures = append(failures, err)
	}
	return errors.Join(failures...)
}

func (p *EVMTrades) resume(ctx context.Context, id uuid.UUID, now time.Time) error {
	err := p.routes.Resume(ctx, ids.SwapIDFrom(id))
	code := errs.CodeOf(err)
	if err == nil || errs.Retryable(code) || ctx.Err() != nil {
		return err
	}
	parkErr := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		row, err := sqlc.New(tx.Queries()).ParkRouted(ctx, sqlc.ParkRoutedParams{At: now, Code: string(code), ID: id})
		if errors.Is(err, sql.ErrNoRows) {
			return nil
		}
		tx.AfterCommit(func(ctx context.Context) {
			observability.Alert(ctx, observability.TradingRoutedSwapParked,
				slog.String("swap_id", id.String()), slog.String("cabal_id", row.CabalID.String()),
				slog.String("code", string(code)), slog.String("step", row.Step))
		})
		return err
	})
	return errors.Join(err, parkErr)
}

func (p *EVMTrades) bridgeBack(
	ctx context.Context, q *sqlc.Queries, row sqlc.ListStrandedCashRow, now time.Time,
) (bool, error) {
	_, err := q.TouchFinished(ctx, sqlc.TouchFinishedParams{ID: row.ID, At: now})
	retryable := false
	held := now.Sub(row.FinishedAt) < StrandedReturnHold
	if err == nil && held && !row.Returning && row.SourceKind == string(domain.SourceProposal) {
		retryable, err = p.proposals.Retryable(ctx, ids.ProposalIDFrom(row.SourceID))
	}
	if err != nil || retryable {
		return false, err
	}
	return true, p.routes.BridgeBack(ctx, ids.SwapIDFrom(row.ID))
}
