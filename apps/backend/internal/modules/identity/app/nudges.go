package app

import (
	"context"
	"errors"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/identity/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const NudgePage = 500

type EmitNudges struct {
	uow      *db.UnitOfWork
	reads    sqlc.DBTX
	clock    clock.Clock
	interval time.Duration
}

func NewEmitNudges(uow *db.UnitOfWork, reads sqlc.DBTX, c clock.Clock, interval time.Duration) *EmitNudges {
	return &EmitNudges{uow: uow, reads: reads, clock: c, interval: interval}
}

func (*EmitNudges) Name() string { return "identity.nudges" }

func (p *EmitNudges) Interval() time.Duration { return p.interval }

func (p *EmitNudges) Tick(ctx context.Context) (poller.Report, error) {
	now := p.clock.Now()
	var (
		report poller.Report
		failed []error
	)
	after := uuid.Nil
	for {
		page, err := sqlc.New(p.reads).NudgeCandidates(ctx, sqlc.NudgeCandidatesParams{
			ChangedBefore: now.Add(-domain.NudgeSettle), NudgedBefore: now.Add(-domain.NudgeGap),
			MaxNudges: domain.MaxNudges, After: after, Page: NudgePage,
		})
		if err != nil {
			return report, errors.Join(append(failed, err)...)
		}
		report.Scanned += len(page)
		nudged, err := p.nudge(ctx, page, now)
		report.Changed += nudged
		failed = append(failed, err)
		if len(page) < NudgePage {
			return report, errors.Join(failed...)
		}
		after = page[len(page)-1]
	}
}

func (p *EmitNudges) nudge(ctx context.Context, page []uuid.UUID, now time.Time) (int, error) {
	if len(page) == 0 {
		return 0, nil
	}
	var nudged int
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		rows, err := sqlc.New(tx.Queries()).MarkNudged(ctx, sqlc.MarkNudgedParams{
			Now: now, Ids: page, ChangedBefore: now.Add(-domain.NudgeSettle), NudgedBefore: now.Add(-domain.NudgeGap),
			MaxNudges: domain.MaxNudges,
		})
		if err != nil {
			return err
		}
		appended := make([]error, 0, len(rows))
		for _, row := range rows {
			appended = append(appended, tx.Events.Append(ctx, events.UserNudgeDue{
				V: 1, UserID: row.ID, Kind: string(domain.NudgeKindFor(domain.AuthState(row.AuthState))),
				NudgeNumber: int(row.NudgeCount), At: now,
			}))
		}
		nudged = len(rows)
		return errors.Join(appended...)
	})
	if err != nil {
		return 0, err
	}
	return nudged, nil
}
