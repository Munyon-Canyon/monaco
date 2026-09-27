package main

import (
	"context"
	"io"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestMain(m *testing.M) {
	testkit.NATSServer(m)
}

func TestRun_refusesToBootWithoutTheBus(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name, url string
		code      errs.Code
		want      string
	}{
		{"unreachable", "nats://127.0.0.1:1", errs.CodeUpstreamUnavailable, "bus.Connect"},
		{"streams missing", testkit.NATSURL(), errs.CodeNotFound, "stream EVENTS: nats: API error: code=404 err_code=10059 description=stream not found; run monacoctl bus apply"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			err := run(t.Context(), io.Discard, []string{
				"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=" + tc.url,
				"MONACO_HTTP_ADDR=127.0.0.1:0", "MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0",
			})
			if errs.CodeOf(err) != tc.code || !strings.Contains(err.Error(), tc.want) {
				t.Fatalf("run = %v, want %s containing %q", err, tc.code, tc.want)
			}
		})
	}
}

func TestRun_bootsWithTheStreamsAppliedAndStopsCleanlyOnCancel(t *testing.T) {
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
	stop := time.AfterFunc(200*time.Millisecond, cancel)
	defer stop.Stop()
	err = run(ctx, io.Discard, []string{
		"MONACO_ENV=test", "DATABASE_URL=postgres://localhost/monaco", "NATS_URL=" + url,
		"MONACO_HTTP_ADDR=127.0.0.1:0", "MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0",
	})
	if err != nil {
		t.Fatalf("run with streams applied = %v, want nil after cancel", err)
	}
	if ctx.Err() == nil {
		t.Fatal("run returned before its context was cancelled")
	}
}
