package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

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

func connectBus(ctx context.Context, cfg config.Config, meters metric.MeterProvider) (*bus.Conn, error) {
	conn, err := bus.Connect(ctx, cfg.NATS, bus.ProcessAPI, bus.WithMeterProvider(meters))
	if err != nil {
		return nil, err
	}
	if err := conn.VerifyStreams(ctx); err != nil {
		drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Timeouts.Shutdown)
		defer cancel()
		conn.Close(drainCtx)
		return nil, err
	}
	return conn, nil
}

func startBackground(
	ctx context.Context, conn *bus.Conn, pool *pgxpool.Pool, uow *db.UnitOfWork, meters metric.MeterProvider,
	relay bool,
) (*sse.Hub, func() error, error) {
	stopRelay := func() error { return nil }
	if relay {
		var err error
		if stopRelay, err = startRelay(ctx, conn, pool, uow, clock.Real{}); err != nil {
			return nil, nil, err
		}
	}
	hub, stopHub, err := startHub(ctx, conn, meters)
	if err != nil {
		return nil, nil, errors.Join(err, stopRelay())
	}
	return hub, func() error {
		stopHub()
		return stopRelay()
	}, nil
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

func startHub(ctx context.Context, conn *bus.Conn, meters metric.MeterProvider) (*sse.Hub, func(), error) {
	hub, err := sse.NewHub(sse.NoMemberships{}, meters)
	if err != nil {
		return nil, nil, err
	}
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		hub.Run(ctx)
		close(done)
	}()
	stop := func() {
		cancel()
		<-done
	}
	if err := conn.SubscribeHints(ctx, hub.Deliver); err != nil {
		stop()
		return nil, nil, err
	}
	return hub, stop, nil
}

func newHandler(
	cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool, verifier auth.TokenVerifier, routes httpx.Routes,
	spec []byte,
) (http.Handler, error) {
	return httpx.Handler(httpx.Deps{
		Logger:       logger,
		Tracer:       otel.GetTracerProvider(),
		Clock:        clock.Real{},
		IDs:          ids.Real{},
		MaxBodyBytes: int64(cfg.HTTP.MaxBodyBytes),
		Idempotency:  db.NewIdempotencyStore(pool, clock.Real{}),
		Verifier:     verifier,
	}, routes, spec)
}

func listen(ctx context.Context, cfg config.Config) (net.Listener, error) {
	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", cfg.HTTP.Addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", cfg.HTTP.Addr, err)
	}
	observability.Info(ctx, observability.BootListening, slog.String("service", "api"),
		slog.String("addr", ln.Addr().String()))
	return ln, nil
}

func bootErr(ctx context.Context, err error) error {
	if ctx.Err() != nil && errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}
