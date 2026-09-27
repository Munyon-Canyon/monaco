package bus

import (
	"context"
	"log/slog"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

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
	nc *nats.Conn
	js jetstream.JetStream
	ns namespace
}

type Option func(*options)

type options struct {
	ns namespace
}

func WithNamespace(ns string) Option {
	return func(o *options) { o.ns = namespace(ns) }
}

func Connect(ctx context.Context, cfg config.NATS, proc Process, opts ...Option) (*Conn, error) {
	const op = "bus.Connect"
	var o options
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
	return &Conn{nc: nc, js: js, ns: o.ns}, nil
}

func (c *Conn) Close(ctx context.Context) {
	closed := make(chan struct{})
	c.nc.SetClosedHandler(func(*nats.Conn) { close(closed) })
	if err := c.nc.Drain(); err != nil {
		c.nc.Close()
		return
	}
	select {
	case <-closed:
	case <-ctx.Done():
		c.nc.Close()
	}
}

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
