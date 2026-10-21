package main

import (
	"context"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const oldestMoneyIntent = `
SELECT min(at) FROM (
  SELECT min(created_at) AS at FROM withdrawals WHERE status IN ('created', 'submitted')
  UNION ALL
  SELECT min(seen_at) FROM deposit_candidates WHERE status = 'pending'
  UNION ALL
  SELECT min(created_at) FROM cash_out_jobs WHERE status NOT IN ('completed', 'partial', 'failed')
  UNION ALL
  SELECT min(created_at) FROM swaps WHERE status IN ('created', 'submitted')
) intents`

type progress struct {
	MoneyIntentOldest time.Duration
	OutboxUnpublished int64
	OutboxOldest      time.Duration
	ConsumerPending   int64
	ConsumerOldest    time.Duration
}

type progressReader interface {
	Progress(ctx context.Context) (progress, error)
}

type dbProgress struct {
	clock     clock.Clock
	intent    func(context.Context) (time.Time, error)
	outbox    func(context.Context) (db.Backlog, error)
	consumers func(context.Context, time.Time) (bus.ConsumerBacklog, error)
}

func newDBProgress(pool *pgxpool.Pool, conn *bus.Conn, c clock.Clock) dbProgress {
	return dbProgress{
		clock: c,
		intent: func(ctx context.Context) (time.Time, error) {
			var oldest *time.Time
			if err := pool.QueryRow(ctx, oldestMoneyIntent).Scan(&oldest); err != nil {
				return time.Time{}, errs.Wrap(err, errs.CodeInternal, "worker.progress")
			}
			if oldest == nil {
				return time.Time{}, nil
			}
			return *oldest, nil
		},
		outbox:    db.NewOutbox(pool, c).Backlog,
		consumers: conn.ConsumerBacklog,
	}
}

func (p dbProgress) Progress(ctx context.Context) (progress, error) {
	now := p.clock.Now()
	oldest, err := p.intent(ctx)
	if err != nil {
		return progress{}, err
	}
	backlog, err := p.outbox(ctx)
	if err != nil {
		return progress{}, err
	}
	consumers, err := p.consumers(ctx, now)
	if err != nil {
		return progress{}, err
	}
	out := progress{
		OutboxUnpublished: backlog.Unpublished, OutboxOldest: backlog.Lag,
		ConsumerPending: consumers.Pending, ConsumerOldest: consumers.Oldest,
	}
	if !oldest.IsZero() {
		out.MoneyIntentOldest = now.Sub(oldest)
	}
	return out, nil
}
