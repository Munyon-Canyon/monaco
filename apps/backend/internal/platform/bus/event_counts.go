package bus

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bucket"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

type CountQuery struct {
	Types   []string
	From    time.Time
	To      time.Time
	Size    bucket.Size
	GroupBy string
	Present string
}

type EventCount struct {
	Start time.Time
	Type  string
	Group string
	Count int64
}

type EventCounts struct {
	q *sqlc.Queries
}

func NewEventCounts(db sqlc.DBTX) EventCounts { return EventCounts{q: sqlc.New(db)} }

func (c EventCounts) CountEvents(ctx context.Context, query CountQuery) ([]EventCount, error) {
	rows, err := c.q.CountEvents(ctx, sqlc.CountEventsParams{
		Bucket: string(query.Size), GroupBy: query.GroupBy, Types: nonNil(query.Types), FromAt: query.From,
		ToAt: query.To, Present: query.Present,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "bus.EventCounts.CountEvents")
	}
	out := make([]EventCount, len(rows))
	for i, r := range rows {
		out[i] = EventCount{Start: r.BucketStart.UTC(), Type: r.Type, Group: r.GroupValue, Count: r.Events}
	}
	return out, nil
}
