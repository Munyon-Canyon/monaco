package main

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"net/http"
	"slices"
	"strings"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

type consumerModule []bus.Consumer

func (consumerModule) Name() string { return "test" }

func (consumerModule) Mount(api.Mount) {}

func (m consumerModule) Consumers() []bus.Consumer { return m }

func (consumerModule) Pollers() []poller.Poller { return nil }

func applyStreams(t *testing.T, url string) {
	t.Helper()
	conn, err := bus.Connect(t.Context(), config.NATS{URL: url}, bus.ProcessMonacoctl)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close(t.Context())
	if _, err := conn.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
}

func healthz(ctx context.Context, addr string) (int, string) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, "http://"+addr+"/healthz", nil)
	if err != nil {
		return 0, err.Error()
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return 0, err.Error()
	}
	defer func() { _ = resp.Body.Close() }()
	body, _ := io.ReadAll(resp.Body)
	return resp.StatusCode, string(body)
}

func listeningAddr(logs *testkit.Logs) string {
	for line := range bytes.Lines(logs.Bytes()) {
		var entry struct {
			Msg  string `json:"msg"`
			Addr string `json:"addr"`
		}
		if json.Unmarshal(line, &entry) == nil && entry.Msg == observability.BootListening.Name {
			return entry.Addr
		}
	}
	return ""
}

func waitHealthy(t *testing.T, logs *testkit.Logs, done <-chan error) string {
	t.Helper()
	var addr string
	waitUntil(t, "the worker to report healthy", func() bool {
		select {
		case err := <-done:
			t.Fatalf("run returned %v before the worker reported healthy:\n%s", err, logs.Bytes())
		default:
		}
		if addr = listeningAddr(logs); addr == "" {
			return false
		}
		code, _ := healthz(t.Context(), addr)
		return code == http.StatusOK
	})
	return addr
}

func TestRun_finishesAnInFlightHandlerAfterSIGTERMAndAcksItBeforeExiting(t *testing.T) {
	t.Parallel()
	url := testkit.StandaloneNATS(t)
	applyStreams(t, url)
	pool, logs := testkit.DB(t), &testkit.Logs{}
	started, release := make(chan struct{}), make(chan struct{})
	slow := bus.Handle("worker.slow", func(ctx context.Context, _ db.Tx, _ events.SystemPinged, _ time.Time) error {
		close(started)
		<-release
		return ctx.Err()
	})
	var mods module.Registry
	mods.Add(func(module.Deps) module.Module {
		return consumerModule{{Durable: "worker-slow", Handlers: []bus.HandlerSpec{slow}}}
	})
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, logs, []string{
			"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=" + url,
			"MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0", "MONACO_TIMEOUT_SHUTDOWN=20s",
		}, noop.NewMeterProvider(), &mods)
	}()
	waitHealthy(t, logs, done)
	err := db.New(pool, ids.Real{}, clock.Real{}).Do(observability.WithActor(t.Context(), "system:test"),
		func(ctx context.Context, tx db.Tx) error {
			return tx.Events.Append(ctx, events.SystemPinged{V: 1, PingID: ids.Real{}.NewV7(), Note: "slow"})
		})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(20 * time.Second):
		t.Fatal("the slow handler never started")
	}
	cancel()
	select {
	case err := <-done:
		t.Fatalf("run returned %v while a handler was still running", err)
	case <-time.After(300 * time.Millisecond):
	}
	close(release)
	if err := <-done; err != nil {
		t.Fatalf("run after SIGTERM = %v, want nil once the in-flight handler finished", err)
	}
	var code string
	if err := pool.QueryRow(t.Context(), `SELECT code FROM event_deliveries WHERE handler = 'worker.slow'`).
		Scan(&code); err != nil || code != "ok" {
		t.Fatalf("delivery row = %q, %v; want ok", code, err)
	}
	if !strings.Contains(
		string(logs.Bytes()),
		`"handler":"worker.slow","subject":"events.system.pinged","outcome":"ack"`,
	) ||
		hasLine(logs, "bus.respond_failed") {
		t.Fatalf("logs lack an acked dispatch of worker.slow, or show a failed ack:\n%s", logs.Bytes())
	}
}

func TestStartConsumers_cancelsAHandlerStillRunningWhenTheShutdownBudgetRunsOut(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	pool := testkit.DB(t)
	uow := db.New(pool, ids.Real{}, clock.Real{})
	started, cancelled := make(chan struct{}), make(chan error, 1)
	stuck := bus.Handle("worker.stuck", func(ctx context.Context, _ db.Tx, _ events.SystemPinged, _ time.Time) error {
		close(started)
		<-ctx.Done()
		cancelled <- ctx.Err()
		return ctx.Err()
	})
	stop, err := startConsumers(t.Context(), b.Conn, uow, clock.Real{},
		[]bus.Consumer{{Durable: "worker-stuck", Handlers: []bus.HandlerSpec{stuck}}})
	if err != nil {
		t.Fatal(err)
	}
	stopRelay, err := startRelay(t.Context(), b.Conn, pool, uow, clock.Real{})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() {
		if err := stopRelay(); err != nil {
			t.Error(err)
		}
	})
	err = uow.Do(observability.WithActor(t.Context(), "system:test"), func(ctx context.Context, tx db.Tx) error {
		return tx.Events.Append(ctx, events.SystemPinged{V: 1, PingID: ids.Real{}.NewV7(), Note: "stuck"})
	})
	if err != nil {
		t.Fatal(err)
	}
	select {
	case <-started:
	case <-time.After(20 * time.Second):
		t.Fatal("the stuck handler never started")
	}
	spent, spend := context.WithCancel(t.Context())
	spend()
	stop(spent)
	if err := <-cancelled; !errors.Is(err, context.Canceled) {
		t.Fatalf("stuck handler context = %v, want cancelled once the budget ran out", err)
	}
}

func TestRun_healthTurnsUnavailableWithinFiveSecondsOfNATSStopping(t *testing.T) {
	t.Parallel()
	url, stopNATS := testkit.StoppableNATS(t)
	applyStreams(t, url)
	dsn, logs := testkit.DB(t).Config().ConnString(), &testkit.Logs{}
	ctx, cancel := context.WithCancel(t.Context())
	defer cancel()
	done := make(chan error, 1)
	go func() {
		done <- run(ctx, logs, []string{
			"MONACO_ENV=test", "DATABASE_URL=" + dsn, "NATS_URL=" + url,
			"MONACO_WORKER_HEALTH_ADDR=127.0.0.1:0", "MONACO_TIMEOUT_SHUTDOWN=2s",
		}, noop.NewMeterProvider(), &module.Registry{})
	}()
	addr := waitHealthy(t, logs, done)
	stopNATS()
	deadline := time.Now().Add(5 * time.Second)
	for {
		code, body := healthz(t.Context(), addr)
		if code == http.StatusServiceUnavailable && strings.HasPrefix(body, "nats disconnected\ndb ok\n") {
			break
		}
		if time.Now().After(deadline) {
			t.Fatalf("GET /healthz 5s after NATS stopped = %d %q, want 503 naming nats", code, body)
		}
		time.Sleep(50 * time.Millisecond)
	}
	cancel()
	if err := <-done; err != nil {
		t.Fatalf("run after NATS stopped = %v, want a clean stop", err)
	}
}

func TestRun_refusesToBootWhenTheSchemaIsBehindNamingTheFix(t *testing.T) {
	t.Parallel()
	pool := testkit.DB(t)
	if _, err := pool.Exec(t.Context(), `DELETE FROM atlas_schema_revisions.atlas_schema_revisions`); err != nil {
		t.Fatal(err)
	}
	err := run(t.Context(), io.Discard, []string{
		"MONACO_ENV=test", "DATABASE_URL=" + pool.Config().ConnString(), "NATS_URL=" + testkit.NATSURL(),
	}, noop.NewMeterProvider(), &module.Registry{})
	hint := slog.String("hint", "run: just migrate db")
	if errs.CodeOf(err) != errs.CodeDBSchemaBehind || !slices.ContainsFunc(errs.Detail(err), hint.Equal) {
		t.Fatalf("run against a database behind the binary = %v (%v), want db_schema_behind with %v",
			err, errs.Detail(err), hint)
	}
}

func TestStartConsumers_reportsAConsumerThatCannotStartOnAClosedConnection(t *testing.T) {
	t.Parallel()
	conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessWorker)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close(t.Context())
	stop, err := startConsumers(t.Context(), conn, nil, clock.Real{}, nil)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || !strings.HasPrefix(err.Error(), "bus.Registry.Start") ||
		stop != nil {
		t.Fatalf("startConsumers on a closed connection = %v (stop set: %v), want upstream_unavailable from Start",
			err, stop != nil)
	}
}
