package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"slices"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const pollerMeter = "github.com/monaco/monaco/apps/backend/internal/platform/poller"

func load(environ []string) (config.Config, error) {
	cfg, err := config.Load(environ)
	if err != nil {
		return config.Config{}, err
	}
	if err := faultpoint.Configure(cfg.Faultpoint); err != nil {
		return config.Config{}, err
	}
	return cfg, nil
}

type shutdown []func(context.Context) error

func (s *shutdown) add(step func(context.Context) error) { *s = append(*s, step) }

func (s shutdown) run(ctx context.Context, budget time.Duration) error {
	ctx, cancel := context.WithTimeout(context.WithoutCancel(ctx), budget)
	defer cancel()
	var err error
	for _, step := range slices.Backward(s) {
		err = errors.Join(err, step(ctx))
	}
	return err
}

func connectBus(ctx context.Context, cfg config.Config, meters metric.MeterProvider) (*bus.Conn, error) {
	conn, err := bus.Connect(ctx, cfg.NATS, bus.ProcessWorker, bus.WithMeterProvider(meters))
	if err != nil {
		return nil, err
	}
	if err := conn.VerifyStreams(ctx); err != nil {
		closeCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Timeouts.Shutdown)
		defer cancel()
		conn.Close(closeCtx)
		return nil, err
	}
	return conn, nil
}

func startRelay(
	ctx context.Context, conn *bus.Conn, pool *pgxpool.Pool, uow *db.UnitOfWork, clk clock.Clock,
) (func() error, error) {
	relay := bus.NewRelay(conn, db.NewOutbox(pool, clk), uow.Signal(), clk)
	unregister, err := relay.ExportBacklogGauges()
	if err != nil {
		return nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		relay.Run(ctx)
	}()
	return func() error {
		cancel()
		<-done
		return unregister()
	}, nil
}

func startWork(
	ctx context.Context, stops *shutdown, meters metric.MeterProvider, mods *module.Registry, d module.Deps,
) (health, error) {
	d.Clock, d.IDs, d.HTTPClient = clock.Real{}, ids.Real{}, httpclient.New
	d.UoW = db.New(d.Pool, d.IDs, d.Clock)
	if err := bindPush(ctx, &d); err != nil {
		return health{}, err
	}
	stopRelay, err := startRelay(context.WithoutCancel(ctx), d.Bus, d.Pool, d.UoW, d.Clock)
	if err != nil {
		return health{}, err
	}
	stops.add(func(context.Context) error { return stopRelay() })
	unregister, err := d.Bus.ExportAccountGauges()
	if err != nil {
		return health{}, err
	}
	stops.add(func(context.Context) error { return unregister() })
	runner, err := poller.NewRunner(d.Pool, d.Clock, meters.Meter(pollerMeter))
	if err != nil {
		return health{}, err
	}
	set := mods.Build(d)
	stopConsumers, err := startConsumers(ctx, d.Bus, d.UoW, d.Clock, set.Consumers(),
		bus.WithAckWait(d.Config.Bus.AckWait))
	if err != nil {
		return health{}, err
	}
	pollers := set.Pollers()
	stopPollers := startPollers(ctx, runner, pollers)
	stops.add(func(ctx context.Context) error {
		stopConsumers(ctx)
		return stopPollers()
	})
	return health{connected: d.Bus.Connected, pool: d.Pool, ticks: runner, pollers: pollers, clock: d.Clock}, nil
}

func startConsumers(
	ctx context.Context, conn *bus.Conn, uow *db.UnitOfWork, clk clock.Clock, consumers []bus.Consumer,
	opts ...bus.RegistryOption,
) (func(context.Context), error) {
	reg, err := bus.NewRegistry(conn, uow, clk, consumers, opts...)
	if err != nil {
		return nil, err
	}
	dispatchCtx, abort := context.WithCancel(context.WithoutCancel(ctx))
	stop, err := reg.Start(dispatchCtx)
	if err != nil {
		abort()
		return nil, err
	}
	return func(budget context.Context) {
		defer abort()
		stopped := make(chan struct{})
		go func() {
			stop()
			close(stopped)
		}()
		select {
		case <-stopped:
		case <-budget.Done():
			abort()
			<-stopped
		}
	}, nil
}

func startPollers(ctx context.Context, runner *poller.Runner, pollers []poller.Poller) func() error {
	ctx, cancel := context.WithCancel(context.WithoutCancel(ctx))
	done := make(chan error, 1)
	go func() { done <- runner.Run(ctx, pollers...) }()
	return func() error {
		cancel()
		return <-done
	}
}

func listen(ctx context.Context, addr string) (net.Listener, error) {
	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", addr, err)
	}
	observability.Info(ctx, observability.BootListening, slog.String("service", "worker"),
		slog.String("addr", ln.Addr().String()))
	return ln, nil
}
