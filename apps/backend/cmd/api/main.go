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

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

func main() {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	err := run(ctx, os.Stderr, os.Environ(), openapi.Spec, otel.GetMeterProvider())
	stop()
	if err != nil {
		ctx := observability.WithLogger(context.Background(), observability.NewLogger(config.Config{}, os.Stderr))
		boundary.Stopped(ctx, "api", err)
		os.Exit(1)
	}
}

func run(
	ctx context.Context, stderr io.Writer, environ []string, spec []byte, meters metric.MeterProvider,
) (err error) {
	cfg, err := load(environ)
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
	pool, err := db.Open(ctx, cfg.DB)
	if err != nil {
		return bootErr(ctx, err)
	}
	defer pool.Close()
	conn, err := connectBus(ctx, cfg, meters)
	if err != nil {
		return bootErr(ctx, err)
	}
	defer func() {
		drainCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), cfg.Timeouts.Shutdown)
		defer cancel()
		conn.Close(drainCtx)
	}()
	uow := db.New(pool, ids.Real{}, clock.Real{})
	hub, stopBackground, err := startBackground(ctx, conn, pool, uow, meters, cfg.Bus.APIRelay)
	if err != nil {
		return bootErr(ctx, err)
	}
	defer func() { err = errors.Join(err, stopBackground()) }()
	handler, err := newHandler(cfg, logger, pool, verifier, registered.Build(module.Deps{
		Config: cfg, Logger: logger, Clock: clock.Real{}, IDs: ids.Real{}, Pool: pool, UoW: uow, Bus: conn,
		HTTPClient: httpclient.New, Hub: hub,
	}).Routes(), spec)
	if err != nil {
		return err
	}
	ln, err := listen(ctx, cfg)
	if err != nil {
		return bootErr(ctx, err)
	}
	return httpx.Serve(ctx, ln, httpx.NewServer(handler, cfg.Timeouts), cfg.Timeouts.Shutdown)
}
