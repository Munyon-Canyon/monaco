package poller

import (
	"context"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

const (
	retentionInterval    = 24 * time.Hour
	deliveryRetention    = 30 * 24 * time.Hour
	idempotencyRetention = 24 * time.Hour
)

type Retention struct {
	pool  *pgxpool.Pool
	clock clock.Clock
}

func NewRetention(pool *pgxpool.Pool, c clock.Clock) *Retention {
	return &Retention{pool: pool, clock: c}
}

func (*Retention) Name() string { return "platform.retention" }

func (*Retention) Interval() time.Duration { return retentionInterval }

func (r *Retention) Tick(ctx context.Context) (Report, error) {
	now := r.clock.Now()
	deliveriesBefore, keysBefore := now.Add(-deliveryRetention), now.Add(-idempotencyRetention)
	deliveries, batches, err := db.PruneDeliveries(ctx, r.pool, deliveriesBefore)
	if err != nil {
		return Report{}, err
	}
	keys, err := db.PruneIdempotencyKeys(ctx, r.pool, keysBefore)
	if err != nil {
		return Report{}, err
	}
	return Report{Scanned: deliveries + keys, Changed: deliveries + keys, Attrs: []slog.Attr{
		slog.GroupAttrs("event_deliveries",
			slog.Int("deleted", deliveries), slog.Time("before", deliveriesBefore), slog.Int("batches", batches)),
		slog.GroupAttrs("idempotency_keys", slog.Int("deleted", keys), slog.Time("before", keysBefore)),
	}}, nil
}
