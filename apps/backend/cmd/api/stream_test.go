package main

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"testing"
	"time"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func freeAddr(t *testing.T) string {
	t.Helper()
	ln, err := new(net.ListenConfig).Listen(t.Context(), "tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatal(err)
	}
	return addr
}

func openStream(ctx context.Context, t *testing.T, url, token string) *http.Response {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
		if err != nil {
			t.Fatal(err)
		}
		req.Header.Set("Authorization", "Bearer "+token)
		resp, err := http.DefaultClient.Do(req)
		if err == nil {
			return resp
		}
		select {
		case <-deadline:
			t.Fatalf("GET %s never answered: %v", url, err)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestRun_streamsHintsFromNATSAndShutsDownWithAStreamOpen(t *testing.T) {
	t.Parallel()
	url := testkit.StandaloneNATS(t)
	conn, err := bus.Connect(t.Context(), config.NATS{URL: url}, bus.ProcessMonacoctl)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(context.Background())
	if _, err := conn.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
	const key = "test-only"
	verifier, err := auth.NewDevVerifier(config.Config{Env: config.EnvTest, Auth: config.Auth{DevTokenKey: key}},
		clock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	user := testkit.NewIDs(477).NewV7().String()
	addr := freeAddr(t)

	dsn := testkit.DB(t).Config().ConnString()
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	stopped := make(chan error, 1)
	go func() {
		stopped <- run(ctx, io.Discard, []string{
			"MONACO_ENV=test", "DATABASE_URL=" + dsn, "NATS_URL=" + url,
			"MONACO_DEV_TOKEN_KEY=" + key, "MONACO_HTTP_ADDR=" + addr, "MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0",
			"MONACO_TIMEOUT_SHUTDOWN=5s",
		}, openapi.Spec)
	}()
	streamCtx, stopStream := context.WithTimeout(t.Context(), 10*time.Second)
	defer stopStream()
	resp := openStream(streamCtx, t, "http://"+addr+"/v1/stream",
		verifier.Mint(user, clock.Real{}.Now().Add(time.Hour)))
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("GET /v1/stream = %d, want 200", resp.StatusCode)
	}

	conn.PublishHint(t.Context(), "user."+user+".balance", nil)
	r := bufio.NewReader(resp.Body)
	for _, want := range []string{"id: 1\n", "event: hint\n", `data: {"key":"user:` + user + `","what":"balance"}` + "\n"} {
		if line, err := r.ReadString('\n'); err != nil || line != want {
			t.Fatalf("stream line = %q, %v; want %q", line, err, want)
		}
	}

	cancel()
	select {
	case err := <-stopped:
		if err != nil {
			t.Fatalf("run with an open stream = %v, want a clean stop", err)
		}
	case <-time.After(10 * time.Second):
		t.Fatal("run did not stop within 10s with a stream open")
	}
}
