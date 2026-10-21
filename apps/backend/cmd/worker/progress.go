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
	pool  *pgxpool.Pool
	bus   *bus.Conn
	clock clock.Clock
}

func (p dbProgress) Progress(ctx context.Context) (progress, error) {
	const op = "worker.progress"
	now := p.clock.Now()
	var oldest *time.Time
	if err := p.pool.QueryRow(ctx, oldestMoneyIntent).Scan(&oldest); err != nil {
		return progress{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	backlog, err := db.NewOutbox(p.pool, p.clock).Backlog(ctx)
	if err != nil {
		return progress{}, err
	}
	consumers, err := p.bus.ConsumerBacklog(ctx, now)
	if err != nil {
		return progress{}, err
	}
	out := progress{
		OutboxUnpublished: backlog.Unpublished, OutboxOldest: backlog.Lag,
		ConsumerPending: consumers.Pending, ConsumerOldest: consumers.Oldest,
	}
	if oldest != nil {
		out.MoneyIntentOldest = now.Sub(*oldest)
	}
	return out, nil
}
