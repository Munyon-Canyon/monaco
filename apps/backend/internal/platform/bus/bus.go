package bus

import (
	"context"
	"log/slog"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

type Process string

const (
	ProcessAPI       Process = "api"
	ProcessWorker    Process = "worker"
	ProcessMonacoctl Process = "monacoctl"
)

type Conn struct {
	nc          *nats.Conn
	js          jetstream.JetStream
	ns          namespace
	meter       metric.Meter
	hintDropped metric.Int64Counter
}

type Option func(*options)

type options struct {
	meters metric.MeterProvider
	ns     namespace
}

func WithMeterProvider(mp metric.MeterProvider) Option {
	return func(o *options) { o.meters = mp }
}

func WithNamespace(ns string) Option {
	return func(o *options) { o.ns = namespace(ns) }
}

func Connect(ctx context.Context, cfg config.NATS, proc Process, opts ...Option) (*Conn, error) {
	const op = "bus.Connect"
	o := options{meters: otel.GetMeterProvider()}
	for _, opt := range opts {
		opt(&o)
	}
	nc, err := nats.Connect(cfg.URL, nats.Name("monaco-"+string(proc)))
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeUpstreamUnavailable, op, slog.String("process", string(proc)))
	}
	js, err := jetstream.New(nc)
	if err == nil {
		_, err = js.AccountInfo(ctx)
	}
	if err != nil {
		nc.Close()
		return nil, errs.Wrap(err, errs.CodeUpstreamUnavailable, op, slog.String("process", string(proc)))
	}
	meter := o.meters.Meter("github.com/monaco/monaco/apps/backend/internal/platform/bus")
	dropped, err := meter.Int64Counter("monaco_bus_hint_dropped_total",
		metric.WithDescription("Core NATS hints that failed to publish."))
	if err != nil {
		nc.Close()
		return nil, errs.Wrap(err, errs.CodeInternal, op)
	}
	return &Conn{nc: nc, js: js, ns: o.ns, meter: meter, hintDropped: dropped}, nil
}

const closeFlushTimeout = 5 * time.Second

func (c *Conn) Close(ctx context.Context) {
	defer c.nc.Close()
	if !c.nc.IsConnected() {
		return
	}
	ctx, cancel := context.WithTimeout(ctx, closeFlushTimeout)
	defer cancel()
	_ = c.nc.FlushWithContext(ctx)
}

func (c *Conn) Connected() bool { return c.nc.IsConnected() }

func (c *Conn) Stream(name string) string { return c.ns.stream(name) }

func (c *Conn) Subject(subject string) string { return c.ns.subject(subject) }

type namespace string

func (ns namespace) stream(name string) string {
	if ns == "" {
		return name
	}
	return string(ns) + "_" + name
}

func (ns namespace) subject(subject string) string {
	if ns == "" {
		return subject
	}
	return string(ns) + "." + subject
}
