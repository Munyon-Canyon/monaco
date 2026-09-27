package db

import (
	"context"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

type Outbox struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

type OutboxRow = sqlc.ListUnpublishedRow

type Batch struct {
	Published []uuid.UUID
	Failed    error
	Full      bool
}

type Backlog struct {
	Unpublished int64
	Lag         time.Duration
}

func NewOutbox(pool *pgxpool.Pool, c clock.Clock) *Outbox {
	return &Outbox{pool: pool, clock: c}
}

func (o *Outbox) Drain(
	ctx context.Context, limit int32, publish func(context.Context, OutboxRow) error,
) (Batch, error) {
	const op = "db.Outbox.Drain"
	pgtx, err := o.pool.Begin(ctx)
	if err != nil {
		return Batch{}, classify(err, op)
	}
	q := sqlc.New(pgtx)
	rows, err := q.ListUnpublished(ctx, limit)
	if err != nil {
		return Batch{}, withRollback(classify(err, op), rollback(ctx, pgtx))
	}
	b := Batch{Full: len(rows) == int(limit)}
	for _, row := range rows {
		if b.Failed = publish(ctx, row); b.Failed != nil {
			break
		}
		b.Published = append(b.Published, row.ID)
	}
	if len(b.Published) > 0 {
		params := sqlc.MarkPublishedParams{PublishedAt: o.clock.Now(), Ids: b.Published}
		if _, err := q.MarkPublished(ctx, params); err != nil {
			return Batch{}, withRollback(classify(err, op), rollback(ctx, pgtx))
		}
	}
	finishCtx, cancel := finishContext(ctx)
	defer cancel()
	if err := pgtx.Commit(finishCtx); err != nil {
		return Batch{}, withRollback(classify(err, op), rollback(ctx, pgtx))
	}
	return b, nil
}

func (o *Outbox) Backlog(ctx context.Context) (Backlog, error) {
	now := o.clock.Now()
	row, err := sqlc.New(o.pool).Backlog(ctx, now)
	if err != nil {
		return Backlog{}, classify(err, "db.Outbox.Backlog")
	}
	return Backlog{Unpublished: row.Unpublished, Lag: now.Sub(row.OldestCreatedAt)}, nil
}
