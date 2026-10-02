package bus

import (
	"context"
	"strings"
	"testing"
	"time"

	natsserver "github.com/nats-io/nats-server/v2/server"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

type consumeFails struct {
	jetstream.Consumer
	err error
}

func (c consumeFails) Consume(jetstream.MessageHandler, ...jetstream.PullConsumeOpt) (jetstream.ConsumeContext, error) {
	return nil, c.err
}

type consumerFromFake struct {
	jetstream.JetStream
	cons jetstream.Consumer
}

func (f consumerFromFake) CreateOrUpdateConsumer(
	context.Context, string, jetstream.ConsumerConfig,
) (jetstream.Consumer, error) {
	return f.cons, nil
}

func (c *Conn) AfterConsumeStop(hook func(jetstream.ConsumeContext)) {
	c.js = stopHookJS{JetStream: c.js, hook: hook}
}

type stopHookJS struct {
	jetstream.JetStream
	hook func(jetstream.ConsumeContext)
}

func (s stopHookJS) CreateOrUpdateConsumer(
	ctx context.Context, stream string, cfg jetstream.ConsumerConfig,
) (jetstream.Consumer, error) {
	cons, err := s.JetStream.CreateOrUpdateConsumer(ctx, stream, cfg)
	if err != nil {
		return nil, err
	}
	return stopHookConsumer{Consumer: cons, hook: s.hook}, nil
}

type stopHookConsumer struct {
	jetstream.Consumer
	hook func(jetstream.ConsumeContext)
}

func (c stopHookConsumer) Consume(
	handler jetstream.MessageHandler, opts ...jetstream.PullConsumeOpt,
) (jetstream.ConsumeContext, error) {
	cc, err := c.Consumer.Consume(handler, opts...)
	if err != nil {
		return nil, err
	}
	return stopHookContext{ConsumeContext: cc, hook: c.hook}, nil
}

type stopHookContext struct {
	jetstream.ConsumeContext
	hook func(jetstream.ConsumeContext)
}

func (c stopHookContext) Stop() {
	c.ConsumeContext.Stop()
	c.hook(c.ConsumeContext)
}

func TestStart_returnsTheConsumeErrorAndDropsTheAdvisorySubscription(t *testing.T) {
	t.Parallel()
	srv, err := natsserver.NewServer(&natsserver.Options{
		Host:      "127.0.0.1",
		Port:      natsserver.RANDOM_PORT,
		NoLog:     true,
		NoSigs:    true,
		JetStream: true,
		StoreDir:  t.TempDir(),
	})
	if err != nil {
		t.Fatal(err)
	}
	srv.Start()
	t.Cleanup(func() { srv.Shutdown(); srv.WaitForShutdown() })
	if !srv.ReadyForConnections(10 * time.Second) {
		t.Fatal("nats-server not ready")
	}
	nc, err := nats.Connect(srv.ClientURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	refused := errs.New(errs.CodeInternal, "test.Consume")
	conn := &Conn{
		nc:    nc,
		js:    consumerFromFake{cons: consumeFails{err: refused}},
		ns:    "t_start",
		meter: noop.NewMeterProvider().Meter("test"),
	}
	reg, err := NewRegistry(
		conn,
		nil,
		clock.Real{},
		[]Consumer{{Durable: "notify", Handlers: []HandlerSpec{{Name: "notify.push"}}}},
	)
	if err != nil {
		t.Fatal(err)
	}

	stop, err := reg.Start(t.Context())
	if stop != nil || errs.CodeOf(err) != errs.CodeUpstreamUnavailable ||
		!strings.Contains(err.Error(), "bus.Registry.Start") ||
		!strings.Contains(err.Error(), "test.Consume") {
		t.Fatalf(
			"Start = stop!=nil:%t, %v; want nil stop and upstream_unavailable wrapping the Consume error",
			stop != nil,
			err,
		)
	}
	if n := nc.NumSubscriptions(); n != 0 {
		t.Fatalf("%d subscriptions left after a failed Start, want the advisory subscription gone", n)
	}
}
