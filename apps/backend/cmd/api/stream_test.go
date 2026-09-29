package main

import (
	"bufio"
	"context"
	"io"
	"net"
	"net/http"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
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
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		t.Fatal(err)
	}
	req.Header.Set("Authorization", "Bearer "+token)
	testkit.Eventually(t, func() bool {
		conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", req.URL.Host)
		if err != nil {
			return false
		}
		_ = conn.Close()
		return true
	}, 10*time.Second)
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("GET %s: %v", url, err)
	}
	return resp
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
		}, openapi.Spec, noop.NewMeterProvider())
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

func TestStartBackground_stopsTheRelayWhenTheHubCannotStart(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	pool := testkit.DB(t)
	hub, stop, err := startBackground(t.Context(), b.Conn, pool, db.New(pool, ids.Real{}, clock.Real{}),
		testkit.FailingGauges{Prefix: "monaco_sse_"}, true)
	if errs.CodeOf(err) != errs.CodeInternal || stop != nil || hub != nil {
		t.Fatalf(
			"startBackground with the hub's instruments failing = %v (stop set: %v); want internal and nothing to stop",
			err,
			stop != nil,
		)
	}
}

func TestStartBackground_withTheRelaySwitchedOffStartsNoRelay(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	pool := testkit.DB(t)
	hub, stop, err := startBackground(t.Context(), b.Conn, pool, db.New(pool, ids.Real{}, clock.Real{}),
		testkit.FailingGauges{Prefix: "monaco_events_"}, false)
	if err != nil || hub == nil {
		t.Fatalf("startBackground without the relay = %v; want the hub and no relay gauges registered", err)
	}
	if err := stop(); err != nil {
		t.Fatal(err)
	}
}

func TestStartHub_failsWhenTheHintSubscriptionCannotBeMade(t *testing.T) {
	t.Parallel()
	conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessAPI)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close(t.Context())
	_, stop, err := startHub(t.Context(), conn, noop.NewMeterProvider())
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || !strings.Contains(err.Error(), "bus.SubscribeHints") ||
		stop != nil {
		t.Fatalf("startHub on a closed connection = %v, want upstream_unavailable from bus.SubscribeHints", err)
	}
}
