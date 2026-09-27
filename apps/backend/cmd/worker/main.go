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

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	err := run(ctx, os.Stderr, os.Environ())
	stop()
	if err != nil {
		ctx := observability.WithLogger(context.Background(), observability.NewLogger(config.Config{}, os.Stderr))
		boundary.Error(ctx, observability.BootStopped, slog.String("service", "worker"), slog.Any("err", err))
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
	ctx = observability.WithLogger(ctx, observability.NewLogger(cfg, stderr))
	observability.Info(
		ctx,
		observability.BootConfig,
		slog.String("service", "worker"),
		slog.Any("config", cfg.Redacted()),
	)
	conn, err := bus.Connect(ctx, cfg.NATS, bus.ProcessWorker)
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
	unregister, err := conn.ExportAccountGauges()
	if err != nil {
		return err
	}
	defer func() { err = errors.Join(err, unregister()) }()
	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", cfg.Worker.HealthAddr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", cfg.Worker.HealthAddr, err)
	}
	observability.Info(ctx, observability.BootListening, slog.String("service", "worker"),
		slog.String("addr", ln.Addr().String()))
	return serve(ctx, ln, cfg.Timeouts)
}

func serve(ctx context.Context, ln net.Listener, timeouts config.Timeouts) error {
	mux := http.NewServeMux()
	mux.HandleFunc("GET /healthz", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = io.WriteString(w, "ok\n")
	})
	srv := &http.Server{
		Handler:           mux,
		ReadHeaderTimeout: timeouts.HTTPServerRead,
		ReadTimeout:       timeouts.HTTPServerRead,
		WriteTimeout:      timeouts.HTTPServerWrite,
	}
	served := make(chan error, 1)
	go func() { served <- srv.Serve(ln) }()
	select {
	case err := <-served:
		return err
	case <-ctx.Done():
	}
	shutdownCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), timeouts.Shutdown)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("shutdown: %w", err)
	}
	if err := <-served; !errors.Is(err, http.ErrServerClosed) {
		return err
	}
	return nil
}
