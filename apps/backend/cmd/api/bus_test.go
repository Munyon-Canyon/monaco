package main

import (
	"bytes"
	"context"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	openapi "github.com/monaco/monaco/apps/backend/api"
	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

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
				"MONACO_ENV=test", "DATABASE_URL=" + testkit.DB(t).Config().ConnString(), "NATS_URL=" + tc.url,
				"MONACO_DEV_TOKEN_KEY=test-only", privyKeyEnv,
				"MONACO_HTTP_ADDR=127.0.0.1:0", "MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0",
			}, openapi.Spec, noop.NewMeterProvider())
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

	pool := testkit.DB(t)
	actor := observability.WithActor(t.Context(), "system:test")
	err = db.New(pool, ids.Real{}, clock.Real{}).Do(actor, func(ctx context.Context, tx db.Tx) error {
		return tx.Events.Append(ctx, events.SystemPinged{V: 1, PingID: ids.Real{}.NewV7(), Note: "boot"})
	})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, io.Discard, []string{
			"MONACO_ENV=test", "MONACO_DEV_TOKEN_KEY=test-only", privyKeyEnv, "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=" + url,
			"MONACO_HTTP_ADDR=127.0.0.1:0", "MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0",
		}, openapi.Spec, noop.NewMeterProvider())
	}()
	outbox := db.NewOutbox(pool, clock.Real{})
	waitUntil(t, "the booted relay publishing the seeded event", func() bool {
		select {
		case err := <-done:
			t.Fatalf("run returned %v before its context was cancelled", err)
		default:
		}
		backlog, err := outbox.Backlog(t.Context())
		return err == nil && backlog.Unpublished == 0
	})
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run with streams applied = %v, want nil after cancel", err)
	}
}

func TestRun_refusesToBootWithoutTheDatabase(t *testing.T) {
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
	err = run(t.Context(), io.Discard, []string{
		"MONACO_ENV=test",
		"MONACO_DEV_TOKEN_KEY=test-only", privyKeyEnv,
		"DATABASE_URL=postgres://127.0.0.1:1/monaco?connect_timeout=2",
		"NATS_URL=" + url,
		"MONACO_HTTP_ADDR=127.0.0.1:0",
		"MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0",
	}, openapi.Spec, noop.NewMeterProvider())
	if errs.CodeOf(err) != errs.CodeDBUnavailable || !strings.Contains(err.Error(), "db.Open") {
		t.Fatalf("run without a database = %v, want db_unavailable from db.Open", err)
	}
}

func hasLine(logs *testkit.Logs, msg string) bool {
	return bytes.Contains(logs.Bytes(), []byte(`"msg":"`+msg+`"`))
}

func waitUntil(t *testing.T, what string, ok func() bool) {
	t.Helper()
	t.Logf("waiting for %s", what)
	testkit.Eventually(t, ok, 20*time.Second)
}

func TestStartRelay_wakesOnACommitThroughTheSharedUnitOfWork(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	pool := testkit.DB(t)
	clk := testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	uow := db.New(pool, ids.Real{}, clk)
	logs := &testkit.Logs{}
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, logs))
	stop, err := startRelay(ctx, b.Conn, pool, uow, clk)
	if err != nil {
		t.Fatal(err)
	}
	defer func() {
		if err := stop(); err != nil {
			t.Error(err)
		}
	}()
	waitUntil(t, "the startup drain", func() bool { return hasLine(logs, "bus.relay.idle") })

	err = uow.Do(observability.WithActor(t.Context(), "system:test"), func(ctx context.Context, tx db.Tx) error {
		return tx.Events.Append(ctx, events.SystemPinged{V: 1, PingID: ids.Real{}.NewV7(), Note: "wake"})
	})
	if err != nil {
		t.Fatal(err)
	}
	outbox := db.NewOutbox(pool, clk)
	waitUntil(t, "the relay publishing the commit", func() bool {
		backlog, err := outbox.Backlog(t.Context())
		return err == nil && backlog.Unpublished == 0
	})
	waitUntil(t, "the relay logging the woken tick", func() bool { return hasLine(logs, "bus.relay.tick") })
	s, err := b.JS.Stream(t.Context(), b.Events)
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs != 1 {
		t.Fatalf("stream holds %d messages after a commit with the clock frozen, want 1 published on the wake",
			info.State.Msgs)
	}
}

func TestRun_refusesToBootWhenTheSchemaIsBehindNamingTheFix(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `DELETE FROM atlas_schema_revisions.atlas_schema_revisions`); err != nil {
		t.Fatal(err)
	}
	err := run(t.Context(), io.Discard, []string{
		"MONACO_ENV=test", "MONACO_DEV_TOKEN_KEY=test-only", privyKeyEnv, "DATABASE_URL=" + pool.Config().ConnString(),
		"NATS_URL=" + testkit.NATSURL(), "MONACO_HTTP_ADDR=127.0.0.1:0",
	}, openapi.Spec, noop.NewMeterProvider())
	hint := slog.String("hint", "run: just migrate db")
	if errs.CodeOf(err) != errs.CodeDBSchemaBehind || !slices.ContainsFunc(errs.Detail(err), hint.Equal) {
		t.Fatalf("run against a database behind the binary = %v (%v), want db_schema_behind with %v",
			err, errs.Detail(err), hint)
	}
}
