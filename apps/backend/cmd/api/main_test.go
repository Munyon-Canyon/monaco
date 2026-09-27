package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
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
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln, timeouts) }()

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
