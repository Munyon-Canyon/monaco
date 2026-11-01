package app

import (
	"cmp"
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
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
	QueueStuckMoneyPrefix  = "stuck_"
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
	Name   string
	Count  int
	Href   string
	Oldest time.Duration
}

type StuckMoneyRow struct {
	Kind    string
	Ref     string
	UserID  uuid.UUID
	CabalID uuid.UUID
	Status  string
	Since   time.Time
	Age     time.Duration
}

type Queues struct {
	Swaps   StuckSwaps
	Events  EventQueue
	Summary QueueSummary
	Money   []money.StuckReader
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
	summaries, err := q.MoneySummaries(ctx, StuckAfter)
	if err != nil {
		return nil, err
	}
	counts := []QueueCount{
		{Name: QueueStuckTxns, Count: stuck, Href: "/v1/admin/queues/stuck-txns"},
		{Name: QueueUnpublishedEvents, Count: unpublished, Href: "/v1/admin/queues/unpublished-events"},
		{Name: QueueDeadLetters, Count: summaries[QueueDeadLetters].Count, Href: "/v1/admin/dead-letters"},
	}
	now := q.Clock.Now()
	for _, kind := range money.StuckKinds() {
		sum := summaries[kind]
		count := QueueCount{Name: QueueStuckMoneyPrefix + kind, Count: sum.Count, Href: MoneyHref(kind)}
		if sum.Count > 0 {
			count.Oldest = now.Sub(sum.Oldest)
		}
		counts = append(counts, count)
	}
	return counts, nil
}

func MoneyHref(kind string) string { return "/v1/admin/queues/stuck-money?kind=" + kind }

func (q Queues) MoneySummaries(ctx context.Context, olderThan time.Duration) (map[string]money.StuckSummary, error) {
	rows, err := q.Summary.Summarize(ctx, olderThan)
	if err != nil {
		return nil, err
	}
	out := make(map[string]money.StuckSummary, len(rows))
	for _, row := range rows {
		out[row.Kind] = row
	}
	return out, nil
}

func (q Queues) StuckMoney(
	ctx context.Context, kind string, olderThan time.Duration, limit int,
) ([]StuckMoneyRow, error) {
	var rows []money.StuckRow
	for _, reader := range q.Money {
		part, err := reader.List(ctx, kind, olderThan, limit)
		if err != nil {
			return nil, err
		}
		rows = append(rows, part...)
	}
	slices.SortStableFunc(rows, func(a, b money.StuckRow) int {
		return cmp.Or(a.Since.Compare(b.Since), cmp.Compare(a.Ref, b.Ref))
	})
	rows = rows[:min(len(rows), limit)]
	now := q.Clock.Now()
	out := make([]StuckMoneyRow, len(rows))
	for i, r := range rows {
		out[i] = StuckMoneyRow{
			Kind: r.Kind, Ref: r.Ref, UserID: r.UserID, CabalID: r.CabalID, Status: r.Status, Since: r.Since,
			Age: now.Sub(r.Since),
		}
	}
	return out, nil
}

func (q Queues) OldestMoneyIntent(ctx context.Context) (time.Duration, error) {
	summaries, err := q.MoneySummaries(ctx, 0)
	if err != nil {
		return 0, err
	}
	var oldest time.Duration
	for kind, sum := range summaries {
		if money.IsIntentKind(kind) && sum.Count > 0 {
			oldest = max(oldest, q.Clock.Now().Sub(sum.Oldest))
		}
	}
	return oldest, nil
}
