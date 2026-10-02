package app

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	retentionInterval = 30 * 24 * time.Hour
	rawPriceWindow    = 7 * 24 * time.Hour
	fiveMinuteWindow  = 90 * 24 * time.Hour
	thinBatch         = 1000
)

type Retention struct {
	uow        *db.UnitOfWork
	clock      clock.Clock
	batchLimit int32
	onBatch    func(after time.Time)
}

func NewRetention(uow *db.UnitOfWork, c clock.Clock) *Retention {
	return &Retention{uow: uow, clock: c}
}

func (*Retention) Name() string { return "market.retention" }

func (*Retention) Interval() time.Duration { return retentionInterval }

func (r *Retention) Tick(ctx context.Context) (poller.Report, error) {
	now := r.clock.Now()
	deleted := 0
	for _, pass := range []struct {
		older  time.Time
		bucket string
	}{
		{now.Add(-rawPriceWindow), "5 minutes"},
		{now.Add(-fiveMinuteWindow), "1 hour"},
	} {
		n, err := r.thin(ctx, pass.older, pass.bucket)
		if err != nil {
			return poller.Report{}, err
		}
		deleted += n
	}
	return poller.Report{Scanned: deleted, Changed: deleted}, nil
}

func (r *Retention) thin(ctx context.Context, olderThan time.Time, bucket string) (int, error) {
	deleted := 0
	var after time.Time
	limit := int32(thinBatch)
	if r.batchLimit > 0 {
		limit = r.batchLimit
	}
	for {
		if r.onBatch != nil {
			r.onBatch(after)
		}
		var stamps []time.Time
		err := r.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			var err error
			stamps, err = sqlc.New(tx.Queries()).ThinPricePoints(ctx, sqlc.ThinPricePointsParams{
				After: after, OlderThan: olderThan, Bucket: bucket, BatchLimit: limit,
			})
			return err
		})
		if err != nil {
			return 0, errs.Wrap(err, errs.CodeOf(err), "market.Retention.thin",
				slog.String("bucket", bucket), slog.Time("older_than", olderThan))
		}
		n := len(stamps)
		deleted += n
		if n == 0 {
			return deleted, nil
		}
		after = slices.MaxFunc(stamps, func(a, b time.Time) int { return a.Compare(b) })
	}
}
