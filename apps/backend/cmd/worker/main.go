package main

import (
	"context"
	"errors"
	"io"
	"log/slog"
	"os"
	"os/signal"
	"syscall"

	"go.opentelemetry.io/otel"
	"go.opentelemetry.io/otel/metric"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	err := run(ctx, os.Stderr, os.Environ(), otel.GetMeterProvider(), &registered)
	stop()
	if err != nil {
		ctx := observability.WithLogger(context.Background(), observability.NewLogger(config.Config{}, os.Stderr))
		boundary.Stopped(ctx, "worker", err)
		os.Exit(1)
	}
}

func run(
	ctx context.Context, stderr io.Writer, environ []string, meters metric.MeterProvider, mods *module.Registry,
) (err error) {
	cfg, err := load(environ)
	if err != nil {
		return err
	}
	flush, err := observability.Setup(ctx, cfg)
	if err != nil {
		return err
	}
	var stops shutdown
	defer func() { err = errors.Join(err, stops.run(ctx, cfg.Timeouts.Shutdown)) }()
	stops.add(flush)
	logger := observability.NewLogger(cfg, stderr)
	ctx = observability.WithLogger(ctx, logger)
	observability.Info(ctx, observability.BootConfig, slog.String("service", "worker"),
		slog.Any("config", cfg.Redacted()))
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return err
	}
	stops.add(func(context.Context) error { pool.Close(); return nil })
	conn, err := connectBus(ctx, cfg, meters)
	if err != nil {
		return err
	}
	stops.add(func(ctx context.Context) error { conn.Close(ctx); return nil })
	h, err := startWork(ctx, &stops, meters, mods, module.Deps{Config: cfg, Logger: logger, Pool: pool, Bus: conn})
	if err != nil {
		return err
	}
	ln, err := listen(ctx, cfg.Worker.HealthAddr)
	if err != nil {
		return err
	}
	return httpx.Serve(ctx, ln, httpx.NewServer(h.mux(), cfg.Timeouts), cfg.Timeouts.Shutdown)
}
