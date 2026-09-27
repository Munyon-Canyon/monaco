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

	"go.opentelemetry.io/otel"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
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
		return err
	}
	defer func() {
		flushCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Timeouts.Shutdown)
		defer cancel()
		err = errors.Join(err, shutdown(flushCtx))
	}()
	logger := observability.NewLogger(cfg, stderr)
	ctx = observability.WithLogger(ctx, logger)
	observability.Info(ctx, observability.BootConfig, slog.String("service", "api"), slog.Any("config", cfg.Redacted()))
	conn, err := bus.Connect(ctx, cfg.NATS, bus.ProcessAPI)
	if err != nil {
		return err
	}
	defer func() {
		drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Timeouts.Shutdown)
		defer cancel()
		conn.Close(drainCtx)
	}()
	if err := conn.VerifyStreams(ctx); err != nil {
		return err
	}
	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", cfg.HTTP.Addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.HTTP.Addr, err)
	}
	observability.Info(ctx, observability.BootListening, slog.String("service", "api"),
		slog.String("addr", ln.Addr().String()))
	handler, err := httpx.Handler(httpx.Deps{
		Logger:       logger,
		Tracer:       otel.GetTracerProvider(),
		Clock:        clock.Real{},
		IDs:          ids.Real{},
		MaxBodyBytes: int64(cfg.HTTP.MaxBodyBytes),
	}, httpx.Health{})
	if err != nil {
		return err
	}
	return serve(ctx, ln, httpx.NewServer(handler, cfg.Timeouts), cfg.Timeouts.Shutdown)
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
