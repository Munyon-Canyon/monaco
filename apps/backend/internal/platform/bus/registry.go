package bus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/attribute"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

const (
	MaxDeliver    = 10
	maxAckPending = 64
)

type Handler[E events.Event] func(ctx context.Context, tx db.Tx, e E, at time.Time) error

type OwnHandler[E events.Event] func(ctx context.Context, d Delivery, e E) error

type HandlerSpec struct {
	Name  string
	typ   events.Type
	fetch func(ctx context.Context, e events.Event) (any, error)
	run   func(ctx context.Context, tx db.Tx, e events.Event, fetched any, at time.Time) error
	own   func(ctx context.Context, d Delivery, e events.Event) error
}

type fetchedResult[R any] struct{ value R }

type noFetchedResult struct{}

func Handle[E events.Event](name string, fn Handler[E]) HandlerSpec {
	var zero E
	return HandlerSpec{
		Name: name,
		typ:  zero.Type(),
		run: func(ctx context.Context, tx db.Tx, e events.Event, _ any, at time.Time) error {
			return fn(ctx, tx, e.(E), at)
		},
	}
}

func HandleFetched[E events.Event, R any](
	name string,
	fetch func(ctx context.Context, e E) (R, error),
	apply func(ctx context.Context, tx db.Tx, e E, result R, at time.Time) error,
) HandlerSpec {
	var zero E
	return HandlerSpec{
		Name: name,
		typ:  zero.Type(),
		fetch: func(ctx context.Context, e events.Event) (any, error) {
			result, err := fetch(ctx, e.(E))
			return fetchedResult[R]{value: result}, err
		},
		run: func(ctx context.Context, tx db.Tx, e events.Event, fetched any, at time.Time) error {
			return apply(ctx, tx, e.(E), fetched.(fetchedResult[R]).value, at)
		},
	}
}

func HandleOwn[E events.Event](name string, fn OwnHandler[E]) HandlerSpec {
	var zero E
	return HandlerSpec{
		Name: name,
		typ:  zero.Type(),
		own: func(ctx context.Context, d Delivery, e events.Event) error {
			return fn(ctx, d, e.(E))
		},
	}
}

func (s HandlerSpec) Type() events.Type { return s.typ }

func (s HandlerSpec) OwnIdempotency() bool { return s.own != nil }

func (s HandlerSpec) Apply(ctx context.Context, tx db.Tx, e events.Event, at time.Time) error {
	if s.own != nil {
		return nil
	}
	fetched, err := s.Fetch(ctx, e)
	if err != nil {
		return err
	}
	return s.ApplyFetched(ctx, tx, e, fetched, at)
}

func (s HandlerSpec) Fetch(ctx context.Context, e events.Event) (any, error) {
	if s.fetch == nil {
		return noFetchedResult{}, nil
	}
	return s.fetch(ctx, e)
}

func (s HandlerSpec) ApplyFetched(
	ctx context.Context,
	tx db.Tx,
	e events.Event,
	fetched any,
	at time.Time,
) error {
	return s.run(ctx, tx, e, fetched, at)
}

func (s HandlerSpec) OnCommit(fn func(ctx context.Context, e events.Event)) HandlerSpec {
	if own := s.own; own != nil {
		s.own = func(ctx context.Context, d Delivery, e events.Event) error {
			if err := own(ctx, d, e); err != nil {
				return err
			}
			fn(ctx, e)
			return nil
		}
		return s
	}
	inner := s.run
	s.run = func(ctx context.Context, tx db.Tx, e events.Event, fetched any, at time.Time) error {
		if err := inner(ctx, tx, e, fetched, at); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) { fn(ctx, e) })
		return nil
	}
	return s
}

func (s HandlerSpec) Before(fn func(ctx context.Context, e events.Event)) HandlerSpec {
	if own := s.own; own != nil {
		s.own = func(ctx context.Context, d Delivery, e events.Event) error {
			fn(ctx, e)
			return own(ctx, d, e)
		}
		return s
	}
	inner := s.run
	s.run = func(ctx context.Context, tx db.Tx, e events.Event, fetched any, at time.Time) error {
		fn(ctx, e)
		return inner(ctx, tx, e, fetched, at)
	}
	return s
}

type Consumer struct {
	Durable   string
	Handlers  []HandlerSpec
	NakDelays []time.Duration
}

func (c Consumer) types() []events.Type {
	set := map[events.Type]struct{}{}
	for _, h := range c.Handlers {
		set[h.typ] = struct{}{}
	}
	return slices.Sorted(maps.Keys(set))
}

func NakSchedule() []time.Duration {
	return []time.Duration{time.Second, 5 * time.Second, 30 * time.Second, 2 * time.Minute, 10 * time.Minute}
}

func (c Consumer) nakDelay(delivery uint64) time.Duration {
	schedule := c.NakDelays
	if len(schedule) == 0 {
		schedule = NakSchedule()
	}
	last := schedule[len(schedule)-1]
	for i, d := range schedule {
		if delivery <= uint64(i+1) {
			return d
		}
	}
	return last
}

type Registry struct {
	conn         *Conn
	uow          *db.UnitOfWork
	clock        clock.Clock
	consumers    map[string]Consumer
	ackWait      time.Duration
	durations    metric.Float64Histogram
	gauges       consumerGauges
	beforeClosed func()
}

type consumerGauges struct {
	pending, ackPending, deadLetters metric.Int64ObservableGauge
}

type RegistryOption func(*Registry)

func WithAckWait(d time.Duration) RegistryOption {
	return func(r *Registry) { r.ackWait = d }
}

func NewRegistry(
	conn *Conn, uow *db.UnitOfWork, clk clock.Clock, consumers []Consumer, opts ...RegistryOption,
) (*Registry, error) {
	const op = "bus.NewRegistry"
	r := &Registry{conn: conn, uow: uow, clock: clk, consumers: make(map[string]Consumer, len(consumers))}
	for _, opt := range opts {
		opt(r)
	}
	var err error
	r.durations, err = conn.meter.Float64Histogram("monaco_bus_handler_duration_seconds",
		metric.WithUnit("s"), metric.WithDescription("Handler wall time per outcome."))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	for _, g := range []struct {
		dst  *metric.Int64ObservableGauge
		name string
		unit string
		desc string
	}{
		{&r.gauges.pending, "monaco_bus_consumer_pending", "{message}", "Messages not yet delivered to the consumer."},
		{&r.gauges.ackPending, "monaco_bus_consumer_ack_pending", "{message}", "Messages delivered and not yet acked."},
		{&r.gauges.deadLetters, "monaco_dead_letters", "{message}", "Messages in DEADLETTER per consumer."},
	} {
		*g.dst, err = conn.meter.Int64ObservableGauge(g.name, metric.WithUnit(g.unit), metric.WithDescription(g.desc))
		if err != nil {
			return nil, errs.Wrap(err, errs.CodeInternal, op)
		}
	}
	handlers := map[string]struct{}{}
	for _, c := range consumers {
		if _, dup := r.consumers[c.Durable]; dup {
			panic(fmt.Sprintf("bus: consumer %s registered twice", c.Durable))
		}
		for _, h := range c.Handlers {
			if _, dup := handlers[h.Name]; dup {
				panic(fmt.Sprintf("bus: handler %s registered twice", h.Name))
			}
			handlers[h.Name] = struct{}{}
		}
		r.consumers[c.Durable] = c
	}
	return r, nil
}

func backOff() []time.Duration {
	return []time.Duration{30 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute}
}

func (r *Registry) consumerConfig(c Consumer) jetstream.ConsumerConfig {
	types := c.types()
	subjects := make([]string, len(types))
	for i, t := range types {
		subjects[i] = r.conn.ns.subject(t.Subject())
	}
	cfg := jetstream.ConsumerConfig{
		Durable:        c.Durable,
		FilterSubjects: subjects,
		DeliverPolicy:  jetstream.DeliverNewPolicy,
		AckPolicy:      jetstream.AckExplicitPolicy,
		MaxDeliver:     MaxDeliver,
		BackOff:        backOff(),
		MaxAckPending:  maxAckPending,
	}
	if r.ackWait > 0 {
		cfg.BackOff = nil
		cfg.AckWait = r.ackWait
	}
	return cfg
}

func (r *Registry) Start(ctx context.Context) (func(), error) {
	const op = "bus.Registry.Start"
	stream := r.conn.ns.stream(StreamEvents)
	contexts := make([]jetstream.ConsumeContext, 0, len(r.consumers))
	advisories, err := r.conn.nc.Subscribe(maxDeliveriesAdvisory(stream), func(msg *nats.Msg) {
		r.forwardAdvisory(ctx, msg)
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeUpstreamUnavailable, op)
	}
	started := map[string]jetstream.Consumer{}
	var unregister func() error
	in := newInflight()
	stop := func() {
		_ = advisories.Unsubscribe()
		if unregister != nil {
			_ = unregister()
		}
		for _, cc := range contexts {
			cc.Stop()
			if r.beforeClosed != nil {
				r.beforeClosed()
			}
			<-cc.Closed()
		}
		in.close()
	}
	for _, durable := range slices.Sorted(maps.Keys(r.consumers)) {
		c := r.consumers[durable]
		attrs := []slog.Attr{slog.String("consumer", durable)}
		cons, err := r.conn.js.CreateOrUpdateConsumer(ctx, stream, r.consumerConfig(c))
		if err != nil {
			stop()
			return nil, errs.Wrap(err, errs.CodeUpstreamUnavailable, op, attrs...)
		}
		started[durable] = cons
		cc, err := cons.Consume(
			func(msg jetstream.Msg) { in.run(func() { r.Dispatch(ctx, durable, msg) }) },
			jetstream.PullMaxMessages(1),
			jetstream.ConsumeErrHandler(
				func(_ jetstream.ConsumeContext, err error) { consumeError(ctx, durable, err) },
			),
		)
		if err != nil {
			stop()
			return nil, errs.Wrap(err, errs.CodeUpstreamUnavailable, op, attrs...)
		}
		contexts = append(contexts, cc)
	}
	reg, err := r.conn.meter.RegisterCallback(func(ctx context.Context, o metric.Observer) error {
		return r.observe(ctx, o, started)
	}, r.gauges.pending, r.gauges.ackPending, r.gauges.deadLetters)
	if err != nil {
		stop()
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	unregister = reg.Unregister
	return stop, nil
}

func (r *Registry) observe(ctx context.Context, o metric.Observer, started map[string]jetstream.Consumer) error {
	const op = "bus.Registry.observe"
	for durable, cons := range started {
		info, err := cons.Info(ctx)
		if err != nil {
			return errs.Wrap(err, errs.CodeUpstreamUnavailable, op, slog.String("consumer", durable))
		}
		set := metric.WithAttributes(attribute.String("consumer", durable))
		o.ObserveInt64(r.gauges.pending, int64(min(info.NumPending, math.MaxInt64)), set)
		o.ObserveInt64(r.gauges.ackPending, int64(info.NumAckPending), set)
	}
	var info *jetstream.StreamInfo
	dead, err := r.conn.js.Stream(ctx, r.conn.ns.stream(StreamDeadLetter))
	if err == nil {
		info, err = dead.Info(ctx, jetstream.WithSubjectFilter(r.conn.ns.subject("deadletter.>")))
	}
	if err != nil {
		return errs.Wrap(err, errs.CodeUpstreamUnavailable, op)
	}
	for subject, n := range info.State.Subjects {
		consumer := subject[strings.LastIndex(subject, ".")+1:]
		o.ObserveInt64(r.gauges.deadLetters, int64(min(n, math.MaxInt64)),
			metric.WithAttributes(attribute.String("consumer", consumer)))
	}
	return nil
}

func maxDeliveriesAdvisory(stream string) string {
	return "$JS.EVENT.ADVISORY.CONSUMER.MAX_DELIVERIES." + stream + ".*"
}

func (r *Registry) forwardAdvisory(ctx context.Context, msg *nats.Msg) {
	consumer := msg.Subject[strings.LastIndex(msg.Subject, ".")+1:]
	var advisory struct {
		StreamSeq uint64 `json:"stream_seq"`
	}
	_ = json.Unmarshal(msg.Data, &advisory)
	letter := DeadLetter{Consumer: consumer, MsgID: consumer + "/" + strconv.FormatUint(advisory.StreamSeq, 10)}
	letter.Advisory = rawJSON(msg.Data)
	r.deadLetter(ctx, consumer, letter, "advisory")
}

func consumeError(ctx context.Context, durable string, err error) {
	boundary.Warn(ctx, observability.BusConsumeError, slog.String("consumer", durable), slog.Any("err", err))
}
