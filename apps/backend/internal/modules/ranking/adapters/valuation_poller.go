package adapters

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	ValuationInterval   = time.Second
	ValuationTickBudget = 90 * time.Second
	valuationOp         = "ranking.ValuationPoller.Tick"
)

type ValuationReads interface {
	LastLeaderboardRun(context.Context) (sqlc.LeaderboardRun, error)
	OldestRankingTrigger(context.Context) (time.Time, error)
}

type ValuationRunner interface {
	Run(ctx context.Context, at time.Time) (app.Valuation, error)
}

type ValuationWriter interface {
	Write(ctx context.Context, valuation app.Valuation, startedAt, finishedAt time.Time) (uuid.UUID, error)
}

type ValuationPoller struct {
	Reads  ValuationReads
	Runner ValuationRunner
	Writer ValuationWriter
	Clock  clock.Clock
}

func (ValuationPoller) Name() string { return "ranking.valuation" }

func (ValuationPoller) Interval() time.Duration { return ValuationInterval }

func (ValuationPoller) TickBudget() time.Duration { return ValuationTickBudget }

func (p ValuationPoller) Tick(ctx context.Context) (poller.Report, error) {
	now := p.Clock.Now()
	last, hasRun, err := p.lastRun(ctx)
	if err != nil {
		return poller.Report{}, err
	}
	oldest, hasTrigger, err := p.oldestTrigger(ctx)
	if err != nil {
		return poller.Report{}, err
	}
	var trigger *time.Time
	if hasTrigger {
		trigger = &oldest
	}
	if (hasRun && !now.After(last.AsOf)) || !domain.ShouldRun(now, last.FinishedAt, trigger) {
		return poller.Report{}, nil
	}
	valuation, err := p.Runner.Run(ctx, now)
	if err != nil {
		return poller.Report{}, stalledIfOverdue(err, now, last, hasRun)
	}
	for _, flagged := range valuation.Flagged {
		observability.Info(ctx, observability.RankingCabalExcluded,
			slog.String("cabal", flagged.CabalID.String()), slog.String("reason", string(errs.CodePricesStale)))
	}
	runID, err := p.Writer.Write(ctx, valuation, now, p.Clock.Now())
	if err != nil {
		return poller.Report{}, stalledIfOverdue(err, now, last, hasRun)
	}
	observability.Info(ctx, observability.RankingRunCompleted, slog.String("run_id", runID.String()),
		slog.Int("rows", len(valuation.Entries)), slog.Int("excluded", valuation.Excluded))
	return poller.Report{
		Scanned: len(valuation.Cabals) + len(valuation.Flagged), Changed: len(valuation.Entries),
		Attrs: []slog.Attr{slog.String("run_id", runID.String())},
	}, nil
}

func stalledIfOverdue(err error, now time.Time, last sqlc.LeaderboardRun, hasRun bool) error {
	if !hasRun || now.Sub(last.FinishedAt) <= 3*domain.RunEvery {
		return err
	}
	return errs.Wrap(err, errs.CodeRankingRunsStalled, valuationOp)
}

func (p ValuationPoller) lastRun(ctx context.Context) (sqlc.LeaderboardRun, bool, error) {
	run, err := p.Reads.LastLeaderboardRun(ctx)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return sqlc.LeaderboardRun{}, false, nil
	case err != nil:
		return sqlc.LeaderboardRun{}, false, errs.Wrap(err, errs.CodeOf(err), valuationOp)
	}
	return run, true, nil
}

func (p ValuationPoller) oldestTrigger(ctx context.Context) (time.Time, bool, error) {
	oldest, err := p.Reads.OldestRankingTrigger(ctx)
	switch {
	case errors.Is(err, pgx.ErrNoRows):
		return time.Time{}, false, nil
	case err != nil:
		return time.Time{}, false, errs.Wrap(err, errs.CodeOf(err), valuationOp)
	}
	return oldest, true, nil
}
