package bus_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	sdkmetric "go.opentelemetry.io/otel/sdk/metric"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"
	"go.opentelemetry.io/otel/trace"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	relayTraceID = "4bf92f3577b34da6a3ce929d0e0e4736"
	relaySpanID  = "00f067aa0ba902b7"
	waitFor      = 20 * time.Second
)

type relayHarness struct {
	bus    testkit.Bus
	reader *sdkmetric.ManualReader
	pool   *pgxpool.Pool
	uow    *db.UnitOfWork
	outbox *db.Outbox
	clock  *testkit.Clock
	ids    *testkit.IDs
	logs   *testkit.Logs
	relay  *bus.Relay
}

func newRelayHarness(t *testing.T) *relayHarness {
	t.Helper()
	reader := sdkmetric.NewManualReader()
	h := &relayHarness{
		bus: testkit.NATS(t, testkit.WithBusOptions(
			bus.WithMeterProvider(sdkmetric.NewMeterProvider(sdkmetric.WithReader(reader))))),
		reader: reader,
		clock:  testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)),
		ids:    testkit.NewIDs(11),
		logs:   &testkit.Logs{},
	}
	h.pool = testkit.DB(t)
	h.uow = db.New(h.pool, h.ids, h.clock)
	h.outbox = db.NewOutbox(h.pool, h.clock)
	h.relay = bus.NewRelay(h.bus.Conn, h.outbox, h.uow.Signal(), h.clock)
	return h
}

func (h *relayHarness) ctx(t *testing.T) context.Context {
	t.Helper()
	traceID, _ := trace.TraceIDFromHex(relayTraceID)
	spanID, _ := trace.SpanIDFromHex(relaySpanID)
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID})
	ctx := observability.WithActor(trace.ContextWithSpanContext(t.Context(), sc), "user:u1")
	return observability.WithLogger(ctx, observability.NewLogger(config.Config{Env: config.EnvTest}, h.logs))
}

func (h *relayHarness) start(t *testing.T) context.CancelFunc {
	t.Helper()
	return h.run(t, h.relay)
}

func (h *relayHarness) run(t *testing.T, r *bus.Relay) context.CancelFunc {
	t.Helper()
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvTest}, h.logs))
	ctx, cancel := context.WithCancel(ctx)
	done := make(chan struct{})
	go func() {
		defer close(done)
		r.Run(ctx)
	}()
	stop := func() {
		cancel()
		<-done
	}
	t.Cleanup(stop)
	return stop
}

func (h *relayHarness) append(t *testing.T, n int, note string) []uuid.UUID {
	t.Helper()
	ids := make([]uuid.UUID, 0, n)
	for range n {
		var id uuid.UUID
		err := h.uow.Do(h.ctx(t), func(ctx context.Context, tx db.Tx) error {
			id = h.ids.NewV7()
			return tx.Events.Append(ctx, events.SystemPinged{V: 1, PingID: id, Note: note})
		})
		if err != nil {
			t.Fatal(err)
		}
		ids = append(ids, id)
	}
	return ids
}

func (h *relayHarness) unpublished(t *testing.T) int64 {
	t.Helper()
	b, err := h.outbox.Backlog(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return b.Unpublished
}

func (h *relayHarness) waitDrained(t *testing.T) {
	t.Helper()
	deadline := time.After(waitFor)
	for h.unpublished(t) != 0 {
		select {
		case <-deadline:
			t.Fatalf("%d rows still unpublished after %s", h.unpublished(t), waitFor)
		case <-time.After(10 * time.Millisecond):
		}
	}
}

func (h *relayHarness) fetch(t *testing.T, n int) []jetstream.Msg {
	t.Helper()
	c, err := h.bus.JS.CreateOrUpdateConsumer(t.Context(), h.bus.Events, h.bus.Consumer)
	if err != nil {
		t.Fatal(err)
	}
	batch, err := c.Fetch(n, jetstream.FetchMaxWait(waitFor))
	if err != nil {
		t.Fatal(err)
	}
	var out []jetstream.Msg
	for m := range batch.Messages() {
		out = append(out, m)
	}
	if batch.Error() != nil || len(out) != n {
		t.Fatalf("fetched %d messages (%v), want %d", len(out), batch.Error(), n)
	}
	return out
}

func (h *relayHarness) lines(t *testing.T, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(h.logs.Bytes()))
	for dec.More() {
		var m map[string]any
		if err := dec.Decode(&m); err != nil {
			t.Fatal(err)
		}
		if m["msg"] == msg {
			out = append(out, m)
		}
	}
	return out
}

func TestRelay_publishesACommitWithinOneSignalAndMarksItAfterTheAck(t *testing.T) {
	t.Parallel()
	h := newRelayHarness(t)
	h.start(t)
	ids := h.append(t, 1, "hi")

	h.waitDrained(t)
	msg := h.fetch(t, 1)[0]
	var got events.SystemPinged
	if err := json.Unmarshal(msg.Data(), &got); err != nil {
		t.Fatal(err)
	}
	if msg.Subject() != h.bus.Conn.Subject("events.system.pinged") || got.PingID != ids[0] ||
		msg.Headers().Get(jetstream.MsgIDHeader) == "" {
		t.Fatalf("published %s %+v headers %v", msg.Subject(), got, msg.Headers())
	}
	h.assertTraceParentFromRow(t, msg, ids[0])
	if n := msgs(t, h.bus); n != 1 {
		t.Fatalf("stream holds %d messages, want 1", n)
	}
	ticks := h.lines(t, "bus.relay.tick")
	if len(ticks) != 1 || ticks[0]["count"] != float64(1) || ticks[0]["first_id"] != ticks[0]["last_id"] ||
		ticks[0]["first_id"] != msg.Headers().Get(jetstream.MsgIDHeader) {
		t.Fatalf("tick lines = %v, want one with count 1 and first_id = last_id = the msg id", ticks)
	}
}

func TestRelay_publishesInIDOrderAndLoopsPastAFullBatch(t *testing.T) {
	t.Parallel()
	h := newRelayHarness(t)
	const n = 150
	h.append(t, n, "hi")
	h.run(t, bus.NewRelay(h.bus.Conn, h.outbox, nil, h.clock))

	h.waitDrained(t)
	fetched := h.fetch(t, n)
	msgIDs := make([]string, 0, len(fetched))
	for _, m := range fetched {
		msgIDs = append(msgIDs, m.Headers().Get(jetstream.MsgIDHeader))
	}
	rows := h.eventIDs(t)
	if strings.Join(msgIDs, ",") != strings.Join(rows, ",") {
		t.Fatalf("stream order %v, want events id order %v", msgIDs, rows)
	}
	if n := msgs(t, h.bus); n != 150 {
		t.Fatalf("stream holds %d messages, want %d", n, 150)
	}
	ticks := h.lines(t, "bus.relay.tick")
	if len(ticks) != 2 || ticks[0]["count"] != float64(100) || ticks[1]["count"] != float64(50) {
		t.Fatalf("tick lines = %v, want counts 100 then 50 with no wake channel and no tick", ticks)
	}
}

func (h *relayHarness) assertTraceParentFromRow(t *testing.T, msg jetstream.Msg, pingID uuid.UUID) {
	t.Helper()
	stored := h.storedTraceParent(t, pingID)
	if stored != "00-"+relayTraceID+"-"+relaySpanID+"-00" ||
		len(msg.Headers()["traceparent"]) != 1 || msg.Headers()["traceparent"][0] != stored {
		t.Fatalf("headers %v, want traceparent %q from the row (the relay ran without a span)", msg.Headers(), stored)
	}
}

func (h *relayHarness) storedTraceParent(t *testing.T, pingID uuid.UUID) string {
	t.Helper()
	var tp string
	err := h.pool.QueryRow(t.Context(), `SELECT trace_parent FROM events WHERE aggregate_id = $1`, pingID).Scan(&tp)
	if err != nil {
		t.Fatal(err)
	}
	return tp
}

func (h *relayHarness) eventIDs(t *testing.T) []string {
	t.Helper()
	rows, err := h.pool.Query(t.Context(), `SELECT id FROM events ORDER BY id`)
	if err != nil {
		t.Fatal(err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id uuid.UUID
		if err := rows.Scan(&id); err != nil {
			t.Fatal(err)
		}
		out = append(out, id.String())
	}
	return out
}

func TestRelay_twoRelaysNeverPublishTheSameRowTwice(t *testing.T) {
	t.Parallel()
	h := newRelayHarness(t)
	const n = 120
	h.append(t, n, "hi")
	published := h.tapPublishes(t)
	h.start(t)
	h.run(t, bus.NewRelay(h.bus.Conn, h.outbox, nil, h.clock))

	h.waitDrained(t)
	got := published(t)
	slices.Sort(got)
	if rows := h.eventIDs(t); !slices.Equal(got, rows) {
		t.Fatalf("two relays published %d messages for %d rows; published twice: %v, never published: %v",
			len(got), len(rows), duplicates(got), missing(rows, got))
	}
	if got := msgs(t, h.bus); got != n {
		t.Fatalf("stream holds %d messages after two relays drained %d rows, want %d", got, n, n)
	}
}

func duplicates(sorted []string) []string {
	var out []string
	for i := 1; i < len(sorted); i++ {
		if sorted[i] == sorted[i-1] {
			out = append(out, sorted[i])
		}
	}
	return slices.Compact(out)
}

func missing(want, got []string) []string {
	var out []string
	for _, id := range want {
		if !slices.Contains(got, id) {
			out = append(out, id)
		}
	}
	return out
}

func (h *relayHarness) tapPublishes(t *testing.T) func(t *testing.T) []string {
	t.Helper()
	nc, err := nats.Connect(testkit.NATSURL())
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nc.Close)
	sub, err := nc.SubscribeSync(h.bus.Conn.Subject("events.>"))
	if err != nil {
		t.Fatal(err)
	}
	if err := nc.Flush(); err != nil {
		t.Fatal(err)
	}
	return func(t *testing.T) []string {
		t.Helper()
		if err := nc.Flush(); err != nil {
			t.Fatal(err)
		}
		pending, _, err := sub.Pending()
		if err != nil {
			t.Fatal(err)
		}
		ids := make([]string, 0, pending)
		for range pending {
			msg, err := sub.NextMsg(waitFor)
			if err != nil {
				t.Fatal(err)
			}
			ids = append(ids, msg.Header.Get(jetstream.MsgIDHeader))
		}
		return ids
	}
}

func TestRelay_aCrashAfterPublishBeforeMarkRepublishesAndTheStreamDedupes(t *testing.T) {
	t.Parallel()
	h := newRelayHarness(t)
	h.append(t, 3, "hi")
	ackLost := errs.New(errs.CodeUpstreamUnavailable, "test.ack_lost")
	b, err := h.outbox.Drain(h.ctx(t), 100, func(ctx context.Context, row db.OutboxRow) error {
		err := h.bus.Conn.Publish(ctx, events.Type(row.Type).Subject(), row.Payload, eventIDOf(t, row.ID))
		if err != nil {
			return err
		}
		return ackLost
	})
	if err != nil || !errors.Is(b.Failed, ackLost) || h.unpublished(t) != 3 || msgs(t, h.bus) != 1 {
		t.Fatalf("publish with the ack lost: %+v, %v, %d unpublished, %d in stream; want 3 unpublished and 1 stored",
			b, err, h.unpublished(t), msgs(t, h.bus))
	}

	h.start(t)
	h.waitDrained(t)
	if got := msgs(t, h.bus); got != 3 {
		t.Fatalf("stream holds %d messages after the republish, want 3 (deduped on Nats-Msg-Id)", got)
	}
}

func eventIDOf(t *testing.T, u uuid.UUID) ids.EventID {
	t.Helper()
	id, err := ids.ParseEventID(u.String())
	if err != nil {
		t.Fatal(err)
	}
	return id
}

func TestRelay_aFullStreamKeepsRowsUnpublishedUntilSpaceFrees(t *testing.T) {
	t.Parallel()
	h := newRelayHarness(t)
	s, err := h.bus.JS.Stream(t.Context(), h.bus.Events)
	if err != nil {
		t.Fatal(err)
	}
	cfg := s.CachedInfo().Config
	tiny := cfg
	tiny.MaxBytes = 1024
	if _, err := h.bus.JS.UpdateStream(t.Context(), tiny); err != nil {
		t.Fatal(err)
	}
	h.append(t, 3, strings.Repeat("x", 600))
	h.start(t)

	h.fetch(t, 1)
	h.clock.Advance(time.Second)
	failed := h.waitLines(t, "bus.relay.publish_failed", 2)
	if failed[0]["code"] != "upstream_unavailable" || failed[0]["level"] != "WARN" || h.unpublished(t) != 2 {
		t.Fatalf("publish_failed lines %v with %d unpublished, want upstream_unavailable warnings and 2 rows waiting",
			failed, h.unpublished(t))
	}
	if _, err := h.bus.JS.UpdateStream(t.Context(), cfg); err != nil {
		t.Fatal(err)
	}
	h.clock.Advance(time.Second)
	h.waitDrained(t)
	if got := msgs(t, h.bus); got != 3 {
		t.Fatalf("stream holds %d messages after space freed, want 3", got)
	}
	ticks := h.lines(t, "bus.relay.tick")
	if len(ticks) != 2 || ticks[0]["count"] != float64(1) || ticks[1]["count"] != float64(2) {
		t.Fatalf("tick lines = %v, want 1 then 2", ticks)
	}
}

func TestRelay_backlogGaugesReadZeroAfterADrainedBacklog(t *testing.T) {
	t.Parallel()
	h := newRelayHarness(t)
	unregister, err := h.relay.ExportBacklogGauges()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unregister() }()
	h.append(t, 2, "hi")
	h.clock.Advance(30 * time.Second)
	if n, lag := gauge(t, h.reader, "monaco_events_unpublished"),
		gauge(t, h.reader, "monaco_events_oldest_unpublished_seconds"); n != 2 || lag != 30 {
		t.Fatalf("backlog gauges = %d rows, %ds lag; want 2 and 30", n, lag)
	}

	h.start(t)
	h.waitDrained(t)
	if n, lag := gauge(t, h.reader, "monaco_events_unpublished"),
		gauge(t, h.reader, "monaco_events_oldest_unpublished_seconds"); n != 0 || lag != 0 {
		t.Fatalf("backlog gauges after drain = %d rows, %ds lag; want zeros", n, lag)
	}
}

func TestRelay_logsAndKeepsRunningWhenTheDatabaseFails(t *testing.T) {
	t.Parallel()
	h := newRelayHarness(t)
	unregister, err := h.relay.ExportBacklogGauges()
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = unregister() }()
	stop := h.start(t)
	h.waitLines(t, "bus.relay.idle", 1)
	h.clock.Advance(time.Minute)
	idle := h.waitLines(t, "bus.relay.idle", 2)
	if idle[0]["level"] != "DEBUG" {
		t.Fatalf("idle line = %v, want DEBUG", idle[0])
	}
	h.pool.Close()
	h.clock.Advance(time.Second)
	failed := h.waitLines(t, "bus.relay.failed", 1)
	if failed[0]["code"] != "internal" || failed[0]["level"] != "WARN" {
		t.Fatalf("failed line = %v, want WARN internal", failed[0])
	}
	h.clock.Advance(time.Second)
	h.waitLines(t, "bus.relay.failed", 2)
	var rm metricdata.ResourceMetrics
	if err := h.reader.Collect(t.Context(), &rm); errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("collect on a closed pool = %v, want internal", err)
	}
	stop()
}

func TestRelay_logsIdleAtMostOnceAMinute(t *testing.T) {
	t.Parallel()
	h := newRelayHarness(t)
	ctx := h.ctx(t)
	for range 60 {
		h.relay.Once(ctx)
		h.clock.Advance(time.Second)
	}
	if n := len(h.lines(t, "bus.relay.idle")); n != 1 {
		t.Fatalf("%d bus.relay.idle lines across 59 idle seconds, want 1", n)
	}
	h.relay.Once(ctx)
	if n := len(h.lines(t, "bus.relay.idle")); n != 2 {
		t.Fatalf("%d bus.relay.idle lines once a minute has passed, want 2", n)
	}
}

func (h *relayHarness) waitLines(t *testing.T, msg string, n int) []map[string]any {
	t.Helper()
	deadline := time.After(waitFor)
	for len(h.lines(t, msg)) < n {
		select {
		case <-deadline:
			t.Fatalf("%d %s lines after %s, want %d", len(h.lines(t, msg)), msg, waitFor, n)
		case <-time.After(10 * time.Millisecond):
		}
	}
	return h.lines(t, msg)
}

func TestRelay_exportBacklogGaugesFailsWhenAnInstrumentCannotBeCreated(t *testing.T) {
	t.Parallel()
	for _, fail := range []string{
		"gauge:monaco_events_unpublished", "gauge:monaco_events_oldest_unpublished_seconds", "callback",
	} {
		conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessWorker,
			bus.WithMeterProvider(failingMeters{fail: fail}))
		if err != nil {
			t.Fatal(err)
		}
		_, err = bus.NewRelay(conn, nil, nil, testkit.NewClock(time.Time{})).ExportBacklogGauges()
		conn.Close(context.Background())
		if errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("ExportBacklogGauges with a failing %s = %v, want internal", fail, err)
		}
	}
}
