package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const (
	StuckAfter       = 5 * time.Minute
	UnpublishedAfter = 30 * time.Second
	MaxOlderThan     = 24 * time.Hour
)

const (
	QueueStuckTxns         = "stuck_txns"
	QueueUnpublishedEvents = "unpublished_events"
	QueueDeadLetters       = "dead_letters"
)

type StuckTxn struct {
	ID          ids.SwapID
	CabalID     ids.CabalID
	Action      string
	Symbol      string
	Status      string
	TxSignature string
	CreatedAt   time.Time
	Age         time.Duration
}

type UnpublishedEvent struct {
	ID            uuid.UUID
	Type          string
	AggregateType string
	AggregateID   uuid.UUID
	CreatedAt     time.Time
	Age           time.Duration
}

type QueueCount struct {
	Name  string
	Count int
	Href  string
}

type Queues struct {
	Swaps   StuckSwaps
	Events  EventQueue
	Letters DeadLetterCounter
	Clock   clock.Clock
}

func ParseOlderThan(raw string, fallback time.Duration) (time.Duration, error) {
	if raw == "" {
		return fallback, nil
	}
	d, err := time.ParseDuration(raw)
	if err != nil || d <= 0 || d > MaxOlderThan {
		return 0, errs.Wrap(err, errs.CodeInvalidInput, "admin.ParseOlderThan", slog.String("older_than", raw))
	}
	return d, nil
}

func (q Queues) StuckTxns(ctx context.Context, olderThan time.Duration, limit int) ([]StuckTxn, error) {
	swaps, err := q.Swaps.Stuck(ctx, olderThan, limit)
	if err != nil {
		return nil, err
	}
	now := q.Clock.Now()
	out := make([]StuckTxn, len(swaps))
	for i, s := range swaps {
		out[i] = StuckTxn{
			ID: s.ID, CabalID: s.CabalID, Action: string(s.Action), Symbol: s.Symbol, Status: string(s.Status),
			TxSignature: string(s.TxSignature), CreatedAt: s.CreatedAt, Age: now.Sub(s.CreatedAt),
		}
	}
	return out, nil
}

func (q Queues) UnpublishedEvents(ctx context.Context, olderThan time.Duration, limit int) ([]UnpublishedEvent, error) {
	rows, err := q.Events.Unpublished(ctx, olderThan, limit)
	if err != nil {
		return nil, err
	}
	now := q.Clock.Now()
	out := make([]UnpublishedEvent, len(rows))
	for i, r := range rows {
		out[i] = UnpublishedEvent{
			ID: r.ID, Type: r.Type, AggregateType: r.AggregateType, AggregateID: r.AggregateID, CreatedAt: r.CreatedAt,
			Age: now.Sub(r.CreatedAt),
		}
	}
	return out, nil
}

func (q Queues) Counts(ctx context.Context) ([]QueueCount, error) {
	stuck, err := q.Swaps.CountStuck(ctx, StuckAfter)
	if err != nil {
		return nil, err
	}
	unpublished, err := q.Events.CountUnpublished(ctx, UnpublishedAfter)
	if err != nil {
		return nil, err
	}
	letters, err := q.Letters.CountOpen(ctx)
	if err != nil {
		return nil, err
	}
	return []QueueCount{
		{Name: QueueStuckTxns, Count: stuck, Href: "/v1/admin/queues/stuck-txns"},
		{Name: QueueUnpublishedEvents, Count: unpublished, Href: "/v1/admin/queues/unpublished-events"},
		{Name: QueueDeadLetters, Count: letters, Href: "/v1/admin/dead-letters"},
	}, nil
}
