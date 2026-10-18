package app

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	retentionInterval = time.Hour
	rawPriceWindow    = 2 * time.Hour
	hourlyPriceWindow = 7 * 24 * time.Hour
	thinBatch         = 1000
)

type Retention struct {
	uow        *db.UnitOfWork
	clock      clock.Clock
	hot        []HotMints
	batchLimit int32
	onBatch    func(after time.Time)
}

func NewRetention(uow *db.UnitOfWork, c clock.Clock, hot ...HotMints) *Retention {
	return &Retention{uow: uow, clock: c, hot: hot}
}

func (*Retention) Name() string { return "market.retention" }

func (*Retention) Interval() time.Duration { return retentionInterval }

func (r *Retention) Tick(ctx context.Context) (poller.Report, error) {
	now := r.clock.Now()
	keep, err := r.keepList(ctx)
	if err != nil {
		return poller.Report{}, err
	}
	passes := []struct {
		name string
		run  func(after time.Time, limit int32) ([]time.Time, error)
		walk bool
	}{
		{"hourly", r.thinner(ctx, now.Add(-rawPriceWindow), "1 hour"), true},
		{"daily", r.keptThinner(ctx, now.Add(-hourlyPriceWindow), "1 day", keep), true},
		{"cold", r.coldDeleter(ctx, now.Add(-hourlyPriceWindow), keep), false},
	}
	deleted := 0
	for _, pass := range passes {
		n, err := r.drain(pass.name, pass.walk, pass.run)
		if err != nil {
			return poller.Report{}, err
		}
		deleted += n
	}
	return poller.Report{Scanned: deleted, Changed: deleted}, nil
}

func (r *Retention) keepList(ctx context.Context) ([]string, error) {
	seen := map[chain.SolanaAddress]bool{}
	keep := []string{}
	for _, read := range r.hot {
		mints, err := read(ctx)
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeOf(err), "market.Retention.keepList")
		}
		for _, m := range mints {
			if !seen[m] {
				seen[m] = true
				keep = append(keep, string(m))
			}
		}
	}
	return keep, nil
}

func (r *Retention) thinner(
	ctx context.Context,
	olderThan time.Time,
	bucket string,
) func(time.Time, int32) ([]time.Time, error) {
	return func(after time.Time, limit int32) ([]time.Time, error) {
		return r.run(ctx, func(q *sqlc.Queries) ([]time.Time, error) {
			return q.ThinPricePoints(ctx, sqlc.ThinPricePointsParams{
				After: after, OlderThan: olderThan, Bucket: bucket, BatchLimit: limit,
			})
		})
	}
}

func (r *Retention) keptThinner(
	ctx context.Context,
	olderThan time.Time,
	bucket string,
	keep []string,
) func(time.Time, int32) ([]time.Time, error) {
	return func(after time.Time, limit int32) ([]time.Time, error) {
		return r.run(ctx, func(q *sqlc.Queries) ([]time.Time, error) {
			return q.ThinKeptPricePoints(ctx, sqlc.ThinKeptPricePointsParams{
				After: after, OlderThan: olderThan, Keep: keep, Bucket: bucket, BatchLimit: limit,
			})
		})
	}
}

func (r *Retention) coldDeleter(
	ctx context.Context,
	olderThan time.Time,
	keep []string,
) func(time.Time, int32) ([]time.Time, error) {
	return func(_ time.Time, limit int32) ([]time.Time, error) {
		return r.run(ctx, func(q *sqlc.Queries) ([]time.Time, error) {
			return q.DeleteColdPricePoints(ctx, sqlc.DeleteColdPricePointsParams{
				OlderThan: olderThan, Keep: keep, BatchLimit: limit,
			})
		})
	}
}

func (r *Retention) run(ctx context.Context, fn func(*sqlc.Queries) ([]time.Time, error)) ([]time.Time, error) {
	var stamps []time.Time
	err := r.uow.Do(ctx, func(_ context.Context, tx db.Tx) error {
		var err error
		stamps, err = fn(sqlc.New(tx.Queries()))
		return err
	})
	return stamps, err
}

func (r *Retention) drain(name string, walk bool, run func(time.Time, int32) ([]time.Time, error)) (int, error) {
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
		stamps, err := run(after, limit)
		if err != nil {
			return 0, errs.Wrap(err, errs.CodeOf(err), "market.Retention.drain", slog.String("pass", name))
		}
		n := len(stamps)
		deleted += n
		if n == 0 {
			return deleted, nil
		}
		if walk {
			after = slices.MaxFunc(stamps, func(a, b time.Time) int { return a.Compare(b) })
		}
	}
}
