package adapters

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	CallBoardInterval   = time.Second
	CallBoardTickBudget = 90 * time.Second
	callBoardOp         = "ranking.CallBoardPoller.Tick"
)

type CallBoardReads interface {
	CallBoardLastRun(context.Context) (sqlc.CallBoardRun, error)
	OldestCallBoardTrigger(context.Context) (time.Time, error)
}

type CallBoardRunner interface {
	Run(ctx context.Context, startedAt time.Time) (int, error)
}

type CallBoardPoller struct {
	Reads  CallBoardReads
	Runner CallBoardRunner
	Clock  clock.Clock
}

func (CallBoardPoller) Name() string { return "ranking.call_board" }

func (CallBoardPoller) Interval() time.Duration { return CallBoardInterval }

func (CallBoardPoller) TickBudget() time.Duration { return CallBoardTickBudget }

func (p CallBoardPoller) Tick(ctx context.Context) (poller.Report, error) {
	now := p.Clock.Now()
	last, err := p.Reads.CallBoardLastRun(ctx)
	hasRun := err == nil
	if err != nil && !errors.Is(err, pgx.ErrNoRows) {
		return poller.Report{}, errs.Wrap(err, errs.CodeOf(err), callBoardOp)
	}
	oldest, err := p.Reads.OldestCallBoardTrigger(ctx)
	var trigger *time.Time
	switch {
	case err == nil:
		trigger = &oldest
	case !errors.Is(err, pgx.ErrNoRows):
		return poller.Report{}, errs.Wrap(err, errs.CodeOf(err), callBoardOp)
	}
	if hasRun && !domain.ShouldRunEvery(now, last.FinishedAt, domain.CallBoardEvery, trigger) {
		return poller.Report{}, nil
	}
	rows, err := p.Runner.Run(ctx, now)
	if err != nil {
		return poller.Report{}, err
	}
	return poller.Report{Changed: rows, Attrs: []slog.Attr{slog.Int("rows", rows)}}, nil
}
