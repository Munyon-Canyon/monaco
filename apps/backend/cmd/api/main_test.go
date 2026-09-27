package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func get(ctx context.Context, url string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	return http.DefaultClient.Do(req)
}

func TestServe_healthzAnswersOkUntilShutdown(t *testing.T) {
	t.Parallel()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	url := "http://" + ln.Addr().String() + "/healthz"
	ctx, cancel := context.WithCancel(t.Context())
	timeouts := config.Timeouts{HTTPServerRead: time.Second, HTTPServerWrite: time.Second, Shutdown: time.Second}
	verifier, err := auth.NewDevVerifier(config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: "k"}},
		clock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	handler, err := httpx.Handler(httpx.Deps{
		Logger: observability.NewLogger(config.Config{}, io.Discard), Tracer: noop.NewTracerProvider(),
		Clock: clock.Real{}, IDs: ids.Real{}, MaxBodyBytes: 1 << 10,
		Idempotency: db.NewIdempotencyStore(testkit.DB(t), clock.Real{}),
		Verifier:    verifier,
	}, routes{})
	if err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	srv := httpx.NewServer(testkit.HTTP(t, handler), timeouts)
	go func() { done <- serve(ctx, ln, srv, timeouts.Shutdown) }()

	resp, err := get(t.Context(), url)
	if err != nil {
		t.Fatal(err)
	}
	body, err := io.ReadAll(resp.Body)
	_ = resp.Body.Close()
	if err != nil {
		t.Fatal(err)
	}
	if resp.StatusCode != http.StatusOK || string(body) != "ok\n" {
		t.Fatalf("GET /healthz = %d %q, want 200 %q", resp.StatusCode, body, "ok\n")
	}

	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned %v after shutdown, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not return within 5s of shutdown")
	}
	if resp, err := get(t.Context(), url); err == nil {
		_ = resp.Body.Close()
		t.Fatal("GET /healthz succeeded after shutdown")
	}
}

func TestRun_cancelledDuringBootStopsCleanly(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	err := run(ctx, io.Discard, []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=" + testkit.NATSURL(),
		"MONACO_HTTP_ADDR=127.0.0.1:0", "MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0", "MONACO_DEV_TOKEN_KEY=test-only",
	})
	if err != nil {
		t.Fatalf("run with a cancelled context = %v, want nil: a stop during boot is a clean stop", err)
	}
}

func TestRun_aShutdownFailureAfterACancelIsReported(t *testing.T) {
	t.Parallel()
	url := testkit.StandaloneNATS(t)
	conn, err := bus.Connect(t.Context(), config.NATS{URL: url}, bus.ProcessMonacoctl)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := conn.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	conn.Close(t.Context())

	ctx, cancel := context.WithCancel(t.Context())
	stop := time.AfterFunc(2*time.Second, cancel)
	defer stop.Stop()
	err = run(ctx, io.Discard, []string{
		"MONACO_ENV=test",
		"DATABASE_URL=" + testkit.DB(t).Config().ConnString(),
		"NATS_URL=" + url,
		"MONACO_HTTP_ADDR=127.0.0.1:0",
		"MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0",
		"OTEL_EXPORTER_OTLP_ENDPOINT=http://127.0.0.1:1",
		"MONACO_TIMEOUT_SHUTDOWN=1s",
		"MONACO_DEV_TOKEN_KEY=test-only",
	})
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || errors.Is(err, context.Canceled) {
		t.Fatalf("run = %v, want the telemetry flush failure reported after the cancel", err)
	}
}

func TestBootErr_onlyOurOwnCancelIsACleanStop(t *testing.T) {
	t.Parallel()
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	foreign := fmt.Errorf("pool: %w", context.Canceled)
	if err := bootErr(cancelled, foreign); err != nil {
		t.Fatalf("bootErr(cancelled ctx, wrapped Canceled) = %v, want nil", err)
	}
	if err := bootErr(t.Context(), foreign); !errors.Is(err, context.Canceled) {
		t.Fatalf(
			"bootErr(live ctx, foreign Canceled) = %v, want the error kept: nobody asked this process to stop",
			err,
		)
	}
	if err := bootErr(cancelled, io.ErrUnexpectedEOF); !errors.Is(err, io.ErrUnexpectedEOF) {
		t.Fatalf("bootErr(cancelled ctx, other error) = %v, want the error kept", err)
	}
}

func TestRun_refusesToBootWithoutRequiredConfig(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), io.Discard, []string{"PATH=/usr/bin"})
	want := "config.Load: invalid_input: missing MONACO_ENV, DATABASE_URL, NATS_URL"
	if err == nil || err.Error() != want || errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("run = %v, want %q", err, want)
	}
}

func TestRun_refusesTheDevVerifierInProduction(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), io.Discard, []string{
		"MONACO_ENV=production", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
		"MONACO_DEV_TOKEN_KEY=dev-only",
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput || !strings.Contains(err.Error(), "auth.NewDevVerifier") {
		t.Fatalf("run = %v, want invalid_input from auth.NewDevVerifier", err)
	}
}

func TestRun_refusesToBootWithMalformedOTelEndpoint(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), io.Discard, []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
		"OTEL_EXPORTER_OTLP_ENDPOINT=collector:4318",
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput || !strings.Contains(err.Error(), "observability.Setup") {
		t.Fatalf("run = %v, want invalid_input from observability.Setup", err)
	}
}
