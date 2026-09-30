package analytics_test

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/chaos"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/posthogfake"
)

const (
	apiKey      = "ph-test-key"
	pingHandler = "analytics.posthog.system.pinged"
)

type msg struct {
	*chaos.Msg
	seen    uint64
	outcome bus.Outcome
	reason  string
	delay   time.Duration
}

func (m *msg) Metadata() (*jetstream.MsgMetadata, error) {
	return &jetstream.MsgMetadata{NumDelivered: m.seen}, nil
}

func (m *msg) Ack() error { m.outcome = bus.OutcomeAck; return nil }

func (m *msg) DoubleAck(context.Context) error { return m.Ack() }

func (m *msg) Nak() error { m.outcome = bus.OutcomeNak; return nil }

func (m *msg) NakWithDelay(d time.Duration) error {
	m.outcome, m.delay = bus.OutcomeNak, d
	return nil
}

func (m *msg) Term() error { m.outcome = bus.OutcomeTerm; return nil }

func (m *msg) TermWithReason(reason string) error {
	m.outcome, m.reason = bus.OutcomeTerm, reason
	return nil
}

type verdict struct {
	outcome bus.Outcome
	reason  string
	delay   time.Duration
}

func (m *msg) verdict() verdict { return verdict{m.outcome, m.reason, m.delay} }

type env struct {
	pool     *pgxpool.Pool
	uow      *db.UnitOfWork
	ids      *testkit.IDs
	clock    *testkit.Clock
	fake     *posthogfake.Fake
	bus      testkit.Bus
	logs     *testkit.Logs
	consumer bus.Consumer
	reg      *bus.Registry
}

type option func(*config.Config)

func withoutAPIKey(c *config.Config) { c.PostHog.APIKey = "" }

func newEnv(t *testing.T, register func(*analytics.Registry), opts ...option) *env {
	t.Helper()
	fake := posthogfake.New(t)
	cfg := config.Config{
		PostHog:  config.PostHog{APIKey: apiKey, Host: fake.Host()},
		Timeouts: config.Timeouts{PostHog: time.Minute},
	}
	for _, opt := range opts {
		opt(&cfg)
	}
	exports := analytics.NewRegistry()
	register(exports)
	consumers := analytics.NewWithExports(module.Deps{Config: cfg, HTTPClient: httpclient.New}, exports).Consumers()
	if len(consumers) != 1 {
		t.Fatalf("module built %d consumers, want 1", len(consumers))
	}
	e := &env{
		pool: testkit.DB(t), ids: testkit.NewIDs(1), clock: testkit.NewClock(clock.Real{}.Now().Truncate(time.Second)),
		fake: fake, bus: testkit.NATS(t), logs: &testkit.Logs{}, consumer: consumers[0],
	}
	e.uow = db.New(e.pool, e.ids, e.clock)
	reg, err := bus.NewRegistry(e.bus.Conn, e.uow, e.clock, consumers)
	if err != nil {
		t.Fatal(err)
	}
	e.reg = reg
	return e
}

func (e *env) ctx(t *testing.T) context.Context {
	t.Helper()
	logger := observability.NewLogger(config.Config{Env: config.EnvLocal}, e.logs)
	return observability.WithLogger(t.Context(), logger)
}

type appended struct {
	id      uuid.UUID
	created time.Time
	ping    events.SystemPinged
	payload []byte
}

func (e *env) append(t *testing.T, actor string) appended {
	t.Helper()
	ping := events.SystemPinged{V: 1, PingID: e.ids.NewV7(), UserID: e.ids.NewV7(), Note: "probe"}
	ctx := observability.WithActor(t.Context(), actor)
	err := e.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, ping) })
	if err != nil {
		t.Fatal(err)
	}
	a := appended{ping: ping}
	err = e.pool.QueryRow(t.Context(), `SELECT id, payload, created_at FROM events WHERE aggregate_id = $1`,
		ping.PingID).Scan(&a.id, &a.payload, &a.created)
	if err != nil {
		t.Fatal(err)
	}
	e.clock.Advance(time.Second)
	return a
}

func (e *env) message(a appended) *msg {
	return &msg{Msg: chaos.NewMsg(e.bus.Conn, events.TypeSystemPinged, ids.EventIDFrom(a.id), a.payload)}
}

func (e *env) deliver(t *testing.T, m *msg) {
	t.Helper()
	m.seen++
	m.outcome, m.reason, m.delay = "", "", 0
	e.reg.Dispatch(e.ctx(t), "analytics", m)
}

func (e *env) deliveries(t *testing.T) int {
	t.Helper()
	var n int
	err := e.pool.QueryRow(t.Context(), `SELECT count(*) FROM event_deliveries WHERE handler = $1`,
		pingHandler).Scan(&n)
	if err != nil {
		t.Fatal(err)
	}
	return n
}

func (e *env) deadLetters(t *testing.T) []bus.DeadLetter {
	t.Helper()
	stream, err := e.bus.JS.Stream(t.Context(), e.bus.DeadLetter)
	if err != nil {
		t.Fatal(err)
	}
	info, err := stream.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if info.State.Msgs == 0 {
		return nil
	}
	raw, err := stream.GetLastMsgForSubject(t.Context(), e.bus.Conn.Subject("deadletter."+e.consumer.Durable))
	if err != nil {
		t.Fatal(err)
	}
	var letter bus.DeadLetter
	if err := json.Unmarshal(raw.Data, &letter); err != nil {
		t.Fatal(err)
	}
	return []bus.DeadLetter{letter}
}

func logLines(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	for dec.More() {
		var line map[string]any
		if err := dec.Decode(&line); err != nil {
			t.Fatal(err)
		}
		out = append(out, line)
	}
	return out
}

func logged(t *testing.T, e *env, message string) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, line := range logLines(t, e.logs.Bytes()) {
		if line["msg"] == message {
			out = append(out, line)
		}
	}
	return out
}

func sameCapture(t *testing.T, got, want fakes.PostHogCapture) {
	t.Helper()
	if !got.Timestamp.Equal(want.Timestamp) {
		t.Errorf("timestamp = %s, want %s", got.Timestamp, want.Timestamp)
	}
	got.Timestamp, want.Timestamp = time.Time{}, time.Time{}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("capture = %+v, want %+v", got, want)
	}
}

func userActor(id uuid.UUID) string { return "user:" + id.String() }
