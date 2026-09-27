package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"go.opentelemetry.io/otel"

	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	err := run(ctx, os.Stderr, os.Environ())
	stop()
	if err != nil {
		ctx := observability.WithLogger(context.Background(), observability.NewLogger(config.Config{}, os.Stderr))
		boundary.Error(ctx, observability.BootStopped, slog.String("service", "api"), slog.Any("err", err))
		os.Exit(1)
	}
}

func run(ctx context.Context, stderr io.Writer, environ []string) (err error) {
	cfg, err := config.Load(environ)
	if err != nil {
		return err
	}
	shutdown, err := observability.Setup(ctx, cfg)
	if err != nil {
		return bootErr(ctx, err)
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Timeouts.Shutdown)
		defer cancel()
		err = errors.Join(err, shutdown(flushCtx))
	}()
	logger := observability.NewLogger(cfg, stderr)
	ctx = observability.WithLogger(ctx, logger)
	verifier, err := auth.NewDevVerifier(cfg, clock.Real{})
	if err != nil {
		return err
	}
	observability.Info(ctx, observability.BootConfig, slog.String("service", "api"), slog.Any("config", cfg.Redacted()))
	conn, err := connectBus(ctx, cfg)
	if err != nil {
		return bootErr(ctx, err)
	}
	defer func() {
		drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Timeouts.Shutdown)
		defer cancel()
		conn.Close(drainCtx)
	}()
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return bootErr(ctx, err)
	}
	defer pool.Close()
	stream, stopBackground, err := startBackground(ctx, conn, pool)
	if err != nil {
		return bootErr(ctx, err)
	}
	defer func() { err = errors.Join(err, stopBackground()) }()
	ln, err := listen(ctx, cfg)
	if err != nil {
		return bootErr(ctx, err)
	}
	handler, err := newHandler(cfg, logger, pool, verifier, stream)
	if err != nil {
		return err
	}
	return serve(ctx, ln, httpx.NewServer(handler, cfg.Timeouts), cfg.Timeouts.Shutdown)
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

func connectBus(ctx context.Context, cfg config.Config) (*bus.Conn, error) {
	conn, err := bus.Connect(ctx, cfg.NATS, bus.ProcessAPI)
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

func listen(ctx context.Context, cfg config.Config) (net.Listener, error) {
	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", cfg.HTTP.Addr)
	if err != nil {
		return nil, fmt.Errorf("listen on %s: %w", cfg.HTTP.Addr, err)
	}
	observability.Info(ctx, observability.BootListening, slog.String("service", "api"),
		slog.String("addr", ln.Addr().String()))
	return ln, nil
}

func newHandler(
	cfg config.Config, logger *slog.Logger, pool *pgxpool.Pool, verifier auth.TokenVerifier, stream sse.Stream,
) (http.Handler, error) {
	return httpx.Handler(httpx.Deps{
		Logger:       logger,
		Tracer:       otel.GetTracerProvider(),
		Clock:        clock.Real{},
		IDs:          ids.Real{},
		MaxBodyBytes: int64(cfg.HTTP.MaxBodyBytes),
		Idempotency:  db.NewIdempotencyStore(pool, clock.Real{}),
		Verifier:     verifier,
	}, routes{Stream: stream})
}

func startBackground(ctx context.Context, conn *bus.Conn, pool *pgxpool.Pool) (sse.Stream, func() error, error) {
	stopRelay, err := startRelay(ctx, conn, pool, db.New(pool, ids.Real{}, clock.Real{}), clock.Real{})
	if err != nil {
		return sse.Stream{}, nil, err
	}
	stream, stopHub, err := startStream(ctx, conn)
	if err != nil {
		return sse.Stream{}, nil, errors.Join(err, stopRelay())
	}
	return stream, func() error {
		stopHub()
		return stopRelay()
	}, nil
}

func startStream(ctx context.Context, conn *bus.Conn) (sse.Stream, func(), error) {
	hub, err := sse.NewHub(sse.NoMemberships{}, otel.GetMeterProvider())
	if err != nil {
		return sse.Stream{}, nil, err
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
		return sse.Stream{}, nil, err
	}
	return sse.NewStream(hub, clock.Real{}), stop, nil
}

type routes struct {
	httpx.Health
	sse.Stream
}

func bootErr(ctx context.Context, err error) error {
	if ctx.Err() != nil && errors.Is(err, context.Canceled) {
		return nil
	}
	return err
}

func serve(ctx context.Context, ln net.Listener, srv *http.Server, shutdownTimeout time.Duration) error {
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), shutdownTimeout)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
