package bus

import (
	"context"
	"log/slog"
	"time"

	"go.opentelemetry.io/otel/metric"
	"go.opentelemetry.io/otel/propagation"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

const (
	relayBatch = 100
	relayPoll  = time.Second
)

type Relay struct {
	conn   *Conn
	outbox *db.Outbox
	wake   <-chan struct{}
	clock  clock.Clock
}

func NewRelay(conn *Conn, outbox *db.Outbox, wake <-chan struct{}, c clock.Clock) *Relay {
	return &Relay{conn: conn, outbox: outbox, wake: wake, clock: c}
}

func (r *Relay) Run(ctx context.Context) {
	ticker := r.clock.NewTicker(relayPoll)
	defer ticker.Stop()
	for {
		if r.drain(ctx) {
			continue
		}
		select {
		case <-ctx.Done():
			return
		case <-r.wake:
		case <-ticker.C():
		}
	}
}

func (r *Relay) drain(ctx context.Context) bool {
	b, err := r.outbox.Drain(ctx, relayBatch, r.publish)
	if err != nil {
		boundary.Warn(ctx, observability.BusRelayFailed,
			slog.String("code", string(errs.CodeOf(err))), slog.Any("err", err))
		return false
	}
	if b.Failed != nil {
		boundary.Warn(ctx, observability.BusRelayPublishFailed,
			slog.String("code", string(errs.CodeOf(b.Failed))), slog.Any("err", b.Failed))
	}
	if len(b.Published) == 0 {
		if b.Failed == nil {
			observability.Debug(ctx, observability.BusRelayIdle)
		}
		return false
	}
	observability.Info(ctx, observability.BusRelayTick, slog.Int("count", len(b.Published)),
		slog.String("first_id", b.Published[0].String()),
		slog.String("last_id", b.Published[len(b.Published)-1].String()))
	return b.Full && b.Failed == nil
}

func (r *Relay) publish(ctx context.Context, row db.OutboxRow) error {
	id := ids.EventIDFrom(row.ID)
	ctx = observability.WithEventID(ctx, id)
	ctx = observability.Extract(ctx, propagation.MapCarrier{"traceparent": row.TraceParent.String})
	if err := r.conn.Publish(ctx, events.Type(row.Type).Subject(), row.Payload, id); err != nil {
		return err
	}
	faultpoint.Hit(ctx, faultpoint.AfterPublish)
	return nil
}

func (r *Relay) ExportBacklogGauges() (func() error, error) {
	const op = "bus.Relay.ExportBacklogGauges"
	unpublished, err := r.conn.meter.Int64ObservableGauge("monaco_events_unpublished",
		metric.WithUnit("{event}"), metric.WithDescription("Events rows the relay has not published yet."))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	oldest, err := r.conn.meter.Int64ObservableGauge("monaco_events_oldest_unpublished_seconds",
		metric.WithUnit("s"), metric.WithDescription("Age of the oldest unpublished events row; relay lag."))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	reg, err := r.conn.meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		backlog, err := r.outbox.Backlog(ctx)
		if err != nil {
			return err
		}
		o.ObserveInt64(unpublished, backlog.Unpublished)
		o.ObserveInt64(oldest, int64(backlog.Lag/time.Second))
		return nil
	}, unpublished, oldest)
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	return reg.Unregister, nil
}
