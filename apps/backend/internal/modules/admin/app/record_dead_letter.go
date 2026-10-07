package app

import (
	"context"
	"encoding/json"
	"log/slog"
	"math"
	"time"

	"github.com/google/uuid"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/admin/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const (
	DeadLettersDurable = "admin_deadletters"
	deadLettersBatch   = 100
	statusOpen         = "open"
	statusRedriven     = "redriven"
	statusResolved     = "resolved"
)

type Event struct{ Subject, ID string }

type EventSource interface {
	EventAt(ctx context.Context, seq uint64) (Event, error)
}

type DeadLetterSource interface {
	PullDeadLetters(
		ctx context.Context, durable string, limit int, fn func(context.Context, bus.DeadLetter) error,
	) (int, error)
}

type RecordDeadLetter struct {
	uow    *db.UnitOfWork
	ids    ids.Generator
	clock  clock.Clock
	events EventSource
}

func NewRecordDeadLetter(uow *db.UnitOfWork, g ids.Generator, c clock.Clock, events EventSource) *RecordDeadLetter {
	return &RecordDeadLetter{uow: uow, ids: g, clock: c, events: events}
}

func (h *RecordDeadLetter) Record(ctx context.Context, letter bus.DeadLetter) (bool, error) {
	if letter.Code == bus.ResolvedCode {
		return h.resolve(ctx, letter)
	}
	origin, err := h.origin(ctx, letter)
	if err != nil {
		return false, err
	}
	body, err := json.Marshal(letter)
	if err != nil {
		return false, errs.Wrap(err, errs.CodeInternal, "admin.RecordDeadLetter")
	}
	seq := int64(min(letter.Seq, math.MaxInt64))
	var row sqlc.UpsertDeadLetterRow
	recorded := false
	err = h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		seen, err := q.DeadLetterSeen(ctx, seq)
		if err != nil || seen {
			return err
		}
		row, err = q.UpsertDeadLetter(ctx, sqlc.UpsertDeadLetterParams{
			ID: h.ids.NewV7(), StreamSeq: seq, Consumer: letter.Consumer, Handler: letter.Handler,
			Subject: origin.Subject, EventID: origin.ID, Code: letterCode(letter), Error: letter.Error, Letter: body,
			SeenAt: h.clock.Now(),
		})
		recorded = err == nil
		return err
	})
	if err != nil || !recorded {
		return false, err
	}
	observability.Info(ctx, observability.AdminDeadLetterRecorded,
		slog.String("consumer", letter.Consumer), slog.String("code", letterCode(letter)),
		slog.String("status", row.Status))
	return true, nil
}

func (h *RecordDeadLetter) resolve(ctx context.Context, marker bus.DeadLetter) (bool, error) {
	eventID, ok := parsedEventID(marker.MsgID)
	if !ok {
		return false, nil
	}
	var rows int64
	err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) (err error) {
		rows, err = sqlc.New(tx.Queries()).ResolveDeadLetter(ctx, sqlc.ResolveDeadLetterParams{
			ResolvedAt: h.clock.Now(), Consumer: marker.Consumer, EventID: eventID,
		})
		return err
	})
	if err != nil || rows == 0 {
		return false, err
	}
	observability.Info(ctx, observability.AdminDeadLetterRecorded,
		slog.String("consumer", marker.Consumer), slog.String("code", bus.ResolvedCode),
		slog.String("status", statusResolved))
	return true, nil
}

func (h *RecordDeadLetter) origin(ctx context.Context, letter bus.DeadLetter) (Event, error) {
	if letter.Advisory == nil {
		return Event{Subject: letter.Subject, ID: validEventID(letter.EventID())}, nil
	}
	var advisory struct {
		StreamSeq uint64 `json:"stream_seq"`
	}
	_ = json.Unmarshal(letter.Advisory, &advisory)
	event, err := h.events.EventAt(ctx, advisory.StreamSeq)
	if errs.CodeOf(err) == errs.CodeNotFound {
		return Event{}, nil
	}
	if err != nil {
		return Event{}, err
	}
	return Event{Subject: event.Subject, ID: validEventID(event.ID)}, nil
}

func parsedEventID(raw string) (uuid.UUID, bool) {
	id, err := uuid.Parse(raw)
	return id, err == nil
}

func validEventID(raw string) string {
	if _, ok := parsedEventID(raw); !ok {
		return ""
	}
	return raw
}

func letterCode(letter bus.DeadLetter) string {
	if letter.Advisory != nil {
		return "max_deliveries"
	}
	return letter.Code
}

type DeadLettersDeps struct {
	Source   DeadLetterSource
	Events   EventSource
	UoW      *db.UnitOfWork
	Reads    sqlc.DBTX
	IDs      ids.Generator
	Clock    clock.Clock
	Meter    metric.Meter
	Interval time.Duration
}

type DeadLettersPoller struct {
	source   DeadLetterSource
	record   *RecordDeadLetter
	interval time.Duration
}

func NewDeadLettersPoller(d DeadLettersDeps) *DeadLettersPoller {
	_, _ = d.Meter.Int64ObservableGauge("monaco_dead_letters",
		metric.WithUnit("{message}"),
		metric.WithDescription("Dead letters not yet resolved per consumer."),
		metric.WithInt64Callback(func(ctx context.Context, o metric.Int64Observer) error {
			live, err := sqlc.New(d.Reads).CountLiveDeadLetters(ctx)
			if err != nil {
				return errs.Wrap(err, errs.CodeDBUnavailable, "admin.DeadLetters.gauge")
			}
			for _, row := range live {
				o.Observe(row.Live, metric.WithAttributes(attribute.String("consumer", row.Consumer)))
			}
			return nil
		}))
	return &DeadLettersPoller{
		source:   d.Source,
		record:   NewRecordDeadLetter(d.UoW, d.IDs, d.Clock, d.Events),
		interval: d.Interval,
	}
}

func (*DeadLettersPoller) Name() string { return "admin.deadletters" }

func (p *DeadLettersPoller) Interval() time.Duration { return p.interval }

func (p *DeadLettersPoller) Tick(ctx context.Context) (poller.Report, error) {
	changed := 0
	scanned, err := p.source.PullDeadLetters(ctx, DeadLettersDurable, deadLettersBatch,
		func(ctx context.Context, letter bus.DeadLetter) error {
			did, err := p.record.Record(ctx, letter)
			if did {
				changed++
			}
			return err
		})
	if err != nil {
		return poller.Report{}, err
	}
	return poller.Report{Scanned: scanned, Changed: changed}, nil
}
