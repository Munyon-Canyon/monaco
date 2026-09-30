package analytics

import (
	"context"
	"log/slog"
	"maps"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	durable      = "analytics"
	handlerName  = "analytics.posthog"
	userActor    = "user"
	systemUser   = "system"
	noProfileKey = "$process_person_profile"
)

type Capture = app.Capture

type Registry struct {
	exports []export
}

type export struct {
	subject string
	handler func(exporter) bus.HandlerSpec
}

func NewRegistry() *Registry { return &Registry{} }

func Export[E events.Event](r *Registry, subject string, m func(context.Context, E) (Capture, bool, error)) {
	var zero E
	if typ := zero.Type(); string(typ) != subject {
		panic("analytics: " + subject + " exported for " + string(typ))
	}
	if slices.ContainsFunc(r.exports, func(e export) bool { return e.subject == subject }) {
		panic("analytics: " + subject + " exported twice")
	}
	r.exports = append(r.exports, export{subject: subject, handler: func(x exporter) bus.HandlerSpec {
		return bus.Handle(handlerName+"."+subject, func(ctx context.Context, tx db.Tx, e E, _ time.Time) error {
			return x.send(ctx, tx, subject, func(ctx context.Context) (Capture, bool, error) { return m(ctx, e) })
		})
	}})
}

func (r *Registry) Consumer(port app.PostHog) (bus.Consumer, bool) {
	if len(r.exports) == 0 {
		return bus.Consumer{}, false
	}
	x := exporter{port: port}
	handlers := make([]bus.HandlerSpec, len(r.exports))
	for i, e := range r.exports {
		handlers[i] = e.handler(x)
	}
	return bus.Consumer{Durable: durable, Handlers: handlers}, true
}

type exporter struct {
	port app.PostHog
}

func (x exporter) send(
	ctx context.Context, tx db.Tx, subject string, mapper func(context.Context) (Capture, bool, error),
) error {
	c, ok, err := mapper(ctx)
	switch {
	case err != nil:
		return err
	case !ok:
		observability.Info(ctx, observability.AnalyticsCaptureSkipped,
			slog.String("reason", "mapper"), slog.String("subject", subject))
		return nil
	}
	if c, err = complete(ctx, tx, c); err != nil {
		return err
	}
	if err := x.port.Capture(ctx, []Capture{c}); err != nil {
		observability.Info(ctx, observability.AnalyticsCaptureFailed, slog.String("event", c.Event),
			slog.String("uuid", c.UUID.String()), slog.String("code", string(errs.CodeOf(err))), slog.Any("err", err))
		return err
	}
	observability.Info(ctx, observability.AnalyticsCaptureSent,
		slog.String("event", c.Event), slog.String("uuid", c.UUID.String()))
	return nil
}

func complete(ctx context.Context, tx db.Tx, c Capture) (Capture, error) {
	const op = "analytics.complete"
	id, err := ids.ParseEventID(observability.EventIDFrom(ctx))
	if err != nil {
		return Capture{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	if c.Event == "" {
		return Capture{}, errs.New(errs.CodeInternal, op, slog.String("event_id", id.String()))
	}
	origin, err := sqlc.New(tx.Queries()).GetEventOrigin(ctx, id.UUID())
	if err != nil {
		return Capture{}, err
	}
	c.UUID, c.Timestamp = id.UUID(), origin.CreatedAt.UTC()
	if c.DistinctID != "" {
		return c, nil
	}
	if origin.ActorType == userActor {
		c.DistinctID = origin.ActorID
		return c, nil
	}
	c.DistinctID = systemUser
	c.Properties = maps.Clone(c.Properties)
	if c.Properties == nil {
		c.Properties = map[string]any{}
	}
	c.Properties[noProfileKey] = false
	return c, nil
}
