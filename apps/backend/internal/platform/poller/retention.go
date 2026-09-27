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
	retentionInterval = 24 * time.Hour
	deliveryRetention = 30 * 24 * time.Hour
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
	before := r.clock.Now().Add(-deliveryRetention)
	deleted, batches, err := db.PruneDeliveries(ctx, r.pool, before)
	if err != nil {
		return Report{}, err
	}
	return Report{Scanned: deleted, Changed: deleted, Attrs: []slog.Attr{
		slog.String("table", "event_deliveries"), slog.Time("before", before), slog.Int("batches", batches),
	}}, nil
}
