package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
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
	handler := httpx.Handler(httpx.Deps{
		Logger: observability.NewLogger(config.Config{}, io.Discard), Tracer: noop.NewTracerProvider(),
		Clock: clock.Real{}, IDs: ids.Real{}, MaxBodyBytes: 1 << 10,
	}, httpx.Health{})
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, httpx.NewServer(handler, timeouts), timeouts.Shutdown) }()

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

func TestRun_refusesToBootWithoutRequiredConfig(t *testing.T) {
	t.Parallel()
	err := run(io.Discard, []string{"PATH=/usr/bin"})
	want := "config.Load: invalid_input: missing MONACO_ENV, DATABASE_URL, NATS_URL"
	if err == nil || err.Error() != want || errs.CodeOf(err) != errs.CodeInvalidInput {
		t.Fatalf("run = %v, want %q", err, want)
	}
}

func TestRun_refusesToBootWithMalformedOTelEndpoint(t *testing.T) {
	t.Parallel()
	err := run(io.Discard, []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=nats://localhost:4222",
		"OTEL_EXPORTER_OTLP_ENDPOINT=collector:4318",
	})
	if errs.CodeOf(err) != errs.CodeInvalidInput || !strings.Contains(err.Error(), "observability.Setup") {
		t.Fatalf("run = %v, want invalid_input from observability.Setup", err)
	}
}
