package main

import (
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"
)

func do(ctx context.Context, t *testing.T, method, url, body string) (int, string) {
	t.Helper()
	req, err := http.NewRequestWithContext(ctx, method, url, strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()
	b, err := io.ReadAll(resp.Body)
	if err != nil {
		t.Fatal(err)
	}
	return resp.StatusCode, string(b)
}

func TestServe_replaysFixturesAcceptsScriptsAndStopsOnCancel(t *testing.T) {
	t.Parallel()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	base := "http://" + ln.Addr().String()
	ctx, cancel := context.WithCancel(t.Context())
	done := make(chan error, 1)
	go func() { done <- serve(ctx, ln) }()

	if code, body := do(t.Context(), t, http.MethodGet, base+"/privy/_health", ""); code != http.StatusOK ||
		!strings.Contains(body, `"upstream": "privy"`) {
		t.Fatalf("GET /privy/_health = %d %q", code, body)
	}
	if code, _ := do(t.Context(), t, http.MethodPost, base+"/_script",
		`{"route":"/privy/_health","action":"hang"}`); code != http.StatusNoContent {
		t.Fatalf("POST /_script = %d, want 204", code)
	}
	cancel()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("serve returned %v, want nil", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("serve did not stop within 5s")
	}
}

func TestRun_reportsAnAddressItCannotListenOn(t *testing.T) {
	t.Parallel()
	err := run(t.Context(), []string{"FAKES_ADDR=256.0.0.1:1"})
	if err == nil || !strings.Contains(err.Error(), "listen on 256.0.0.1:1") {
		t.Fatalf("run = %v, want a listen error", err)
	}
}
