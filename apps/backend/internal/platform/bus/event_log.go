package bus

import (
	"context"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db/sqlc"
)

type EventRow struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	Type          string
	Payload       []byte
	ActorType     string
	ActorID       string
	CreatedAt     time.Time
	PublishedAt   *time.Time
}

type StaleEvent struct {
	ID            uuid.UUID
	AggregateType string
	AggregateID   uuid.UUID
	Type          string
	CreatedAt     time.Time
}

type EventLog struct {
	q     *sqlc.Queries
	clock clock.Clock
}

func NewEventLog(db sqlc.DBTX, c clock.Clock) EventLog {
	return EventLog{q: sqlc.New(db), clock: c}
}

func (l EventLog) EventsByAggregate(
	ctx context.Context, aggregateType string, id uuid.UUID, types []string, limit int,
) ([]EventRow, error) {
	rows, err := l.q.EventsByAggregate(ctx, sqlc.EventsByAggregateParams{
		AggregateType: aggregateType, AggregateID: id, Types: nonNil(types), RowLimit: int64(limit),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "bus.EventLog.EventsByAggregate")
	}
	out := make([]EventRow, len(rows))
	for i, r := range rows {
		out[i] = EventRow{
			ID: r.ID, AggregateType: r.AggregateType, AggregateID: r.AggregateID, Type: r.Type,
			Payload: r.Payload, ActorType: r.ActorType, ActorID: r.ActorID, CreatedAt: r.CreatedAt.UTC(),
		}
		if r.PublishedAt.Valid {
			at := r.PublishedAt.Time.UTC()
			out[i].PublishedAt = &at
		}
	}
	return out, nil
}

func (l EventLog) Unpublished(ctx context.Context, olderThan time.Duration, limit int) ([]StaleEvent, error) {
	rows, err := l.q.ListStaleUnpublished(ctx, sqlc.ListStaleUnpublishedParams{
		Cutoff: l.clock.Now().Add(-olderThan), RowLimit: int64(limit),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeDBUnavailable, "bus.EventLog.Unpublished")
	}
	out := make([]StaleEvent, len(rows))
	for i, r := range rows {
		out[i] = StaleEvent{
			ID:            r.ID,
			AggregateType: r.AggregateType,
			AggregateID:   r.AggregateID,
			Type:          r.Type,
			CreatedAt:     r.CreatedAt.UTC(),
		}
	}
	return out, nil
}

func (l EventLog) CountUnpublished(ctx context.Context, olderThan time.Duration) (int, error) {
	n, err := l.q.CountStaleUnpublished(ctx, l.clock.Now().Add(-olderThan))
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeDBUnavailable, "bus.EventLog.CountUnpublished")
	}
	return int(n), nil
}

func nonNil(s []string) []string {
	if s == nil {
		return []string{}
	}
	return s
}
