package bus

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

const (
	maxDeliver    = 10
	maxAckPending = 64
)

type Handler[E events.Event] func(ctx context.Context, tx db.Tx, e E) error

type HandlerSpec struct {
	Name string
	typ  events.Type
	run  func(ctx context.Context, tx db.Tx, e events.Event) error
}

func Handle[E events.Event](name string, fn Handler[E]) HandlerSpec {
	var zero E
	return HandlerSpec{
		Name: name,
		typ:  zero.Type(),
		run: func(ctx context.Context, tx db.Tx, e events.Event) error {
			return fn(ctx, tx, e.(E))
		},
	}
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
	conn      *Conn
	uow       *db.UnitOfWork
	clock     clock.Clock
	consumers map[string]Consumer
	ackWait   time.Duration
}

type RegistryOption func(*Registry)

func WithAckWait(d time.Duration) RegistryOption {
	return func(r *Registry) { r.ackWait = d }
}

func NewRegistry(
	conn *Conn, uow *db.UnitOfWork, clk clock.Clock, consumers []Consumer, opts ...RegistryOption,
) *Registry {
	r := &Registry{conn: conn, uow: uow, clock: clk, consumers: make(map[string]Consumer, len(consumers))}
	for _, opt := range opts {
		opt(r)
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
	return r
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
		MaxDeliver:     maxDeliver,
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
	stop := func() {
		_ = advisories.Unsubscribe()
		for _, cc := range contexts {
			cc.Stop()
			<-cc.Closed()
		}
	}
	for _, durable := range slices.Sorted(maps.Keys(r.consumers)) {
		c := r.consumers[durable]
		attrs := []slog.Attr{slog.String("consumer", durable)}
		cons, err := r.conn.js.CreateOrUpdateConsumer(ctx, stream, r.consumerConfig(c))
		if err != nil {
			stop()
			return nil, errs.Wrap(err, errs.CodeUpstreamUnavailable, op, attrs...)
		}
		cc, err := cons.Consume(
			func(msg jetstream.Msg) { r.Dispatch(ctx, durable, msg) },
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
	return stop, nil
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
	letter := deadLetter{Consumer: consumer, MsgID: consumer + "/" + strconv.FormatUint(advisory.StreamSeq, 10)}
	letter.Advisory = rawJSON(msg.Data)
	r.deadLetter(ctx, consumer, letter, "advisory")
}

func consumeError(ctx context.Context, durable string, err error) {
	boundary.Warn(ctx, observability.BusConsumeError, slog.String("consumer", durable), slog.Any("err", err))
}
