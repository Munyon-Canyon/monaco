package main

import (
	"context"
	"errors"
	"fmt"
	"log/slog"
	"net"
	"net/http"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
)

const defaultAddr = "127.0.0.1:8099"

func main() {
	if !start() {
		os.Exit(1)
	}
}

func start() bool {
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGTERM, os.Interrupt)
	defer stop()
	ctx = observability.WithLogger(ctx, observability.NewLogger(config.Config{}, os.Stderr))
	if err := run(ctx, os.Environ()); err != nil {
		boundary.Error(ctx, observability.BootStopped, slog.String("service", "fakes"), slog.Any("err", err))
		return false
	}
	return true
}

func run(ctx context.Context, environ []string) error {
	addr := defaultAddr
	for _, kv := range environ {
		if v, ok := strings.CutPrefix(kv, "FAKES_ADDR="); ok && v != "" {
			addr = v
		}
	}
	ln, err := new(net.ListenConfig).Listen(ctx, "tcp", addr)
	if err != nil {
		return fmt.Errorf("listen on %s: %w", addr, err)
	}
	observability.Info(ctx, observability.BootListening, slog.String("service", "fakes"),
		slog.String("addr", ln.Addr().String()))
	return serve(ctx, ln)
}

func serve(ctx context.Context, ln net.Listener) error {
	srv := &http.Server{Handler: fakes.New(), ReadHeaderTimeout: 5 * time.Second}
	closed := make(chan error, 1)
	stop := context.AfterFunc(ctx, func() { closed <- srv.Close() })
	defer stop()
	if err := srv.Serve(ln); !errors.Is(err, http.ErrServerClosed) {
		return fmt.Errorf("serve: %w", err)
	}
	if err := <-closed; err != nil {
		return fmt.Errorf("close: %w", err)
	}
	return nil
}
