package bus_test

import (
	"bytes"
	"context"
	"encoding/json"
	"slices"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

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
	durable  = "notify"
	waitLong = 30 * time.Second
)

type lockedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *lockedBuffer) Write(p []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(p)
}

func (b *lockedBuffer) bytes() []byte {
	b.mu.Lock()
	defer b.mu.Unlock()
	return bytes.Clone(b.buf.Bytes())
}

type harness struct {
	bus   testkit.Bus
	pool  *pgxpool.Pool
	uow   *db.UnitOfWork
	ids   *testkit.IDs
	clock *testkit.Clock
	logs  *lockedBuffer
}

func newHarness(t *testing.T) *harness {
	t.Helper()
	h := &harness{
		bus:   testkit.NATS(t),
		pool:  testkit.DB(t),
		ids:   testkit.NewIDs(1),
		clock: testkit.NewClock(time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)),
		logs:  &lockedBuffer{},
	}
	h.uow = db.New(h.pool, h.ids, h.clock)
	_, err := h.pool.Exec(t.Context(), `CREATE TABLE handled (handler text NOT NULL, event_id uuid NOT NULL)`)
	if err != nil {
		t.Fatal(err)
	}
	return h
}

func (h *harness) ctx(t *testing.T) context.Context {
	t.Helper()
	ctx := observability.WithActor(t.Context(), "system:test")
	return observability.WithLogger(ctx, observability.NewLogger(config.Config{Env: config.EnvTest}, h.logs))
}

func (h *harness) registry(t *testing.T, consumers ...bus.Consumer) *bus.Registry {
	t.Helper()
	return bus.NewRegistry(h.bus.Conn, h.uow, h.clock, consumers, bus.WithAckWait(testkit.DefaultAckWait))
}

func (h *harness) recorder(name string) bus.HandlerSpec {
	return bus.Handle(name, func(ctx context.Context, tx db.Tx, e events.SystemPinged) error {
		_, err := tx.Queries().Exec(ctx, `INSERT INTO handled (handler, event_id) VALUES ($1, $2)`, name, e.PingID)
		return err
	})
}

func (h *harness) appendPing(t *testing.T) (uuid.UUID, []byte) {
	t.Helper()
	ev := events.SystemPinged{V: 1, PingID: h.ids.NewV7(), Note: "hi"}
	err := h.uow.Do(h.ctx(t), func(ctx context.Context, tx db.Tx) error { return tx.Events.Append(ctx, ev) })
	if err != nil {
		t.Fatal(err)
	}
	var id uuid.UUID
	var payload []byte
	err = h.pool.QueryRow(t.Context(), `SELECT id, payload FROM events WHERE aggregate_id = $1`, ev.PingID).
		Scan(&id, &payload)
	if err != nil {
		t.Fatal(err)
	}
	return id, payload
}

func (h *harness) publish(t *testing.T, id uuid.UUID, payload []byte) {
	t.Helper()
	eventID, err := ids.ParseEventID(id.String())
	if err != nil {
		t.Fatal(err)
	}
	if err := h.bus.Conn.Publish(t.Context(), events.TypeSystemPinged.Subject(), payload, eventID); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) publishPing(t *testing.T) uuid.UUID {
	t.Helper()
	id, payload := h.appendPing(t)
	h.publish(t, id, payload)
	return id
}

func (h *harness) consumer(t *testing.T) jetstream.Consumer {
	t.Helper()
	cfg := h.bus.Consumer
	cfg.Durable = durable
	cons, err := h.bus.JS.CreateOrUpdateConsumer(t.Context(), h.bus.Events, cfg)
	if err != nil {
		t.Fatal(err)
	}
	return cons
}

func (h *harness) fetch(t *testing.T, cons jetstream.Consumer) jetstream.Msg {
	t.Helper()
	batch, err := cons.Fetch(1, jetstream.FetchMaxWait(waitLong))
	if err != nil {
		t.Fatal(err)
	}
	for msg := range batch.Messages() {
		return msg
	}
	t.Fatalf("fetch returned no message: %v", batch.Error())
	return nil
}

type row struct {
	handler string
	eventID uuid.UUID
	code    string
}

func (h *harness) rows(t *testing.T, query string) []row {
	t.Helper()
	rs, err := h.pool.Query(t.Context(), query)
	if err != nil {
		t.Fatal(err)
	}
	defer rs.Close()
	var out []row
	for rs.Next() {
		var r row
		if err := rs.Scan(&r.handler, &r.eventID, &r.code); err != nil {
			t.Fatal(err)
		}
		out = append(out, r)
	}
	if err := rs.Err(); err != nil {
		t.Fatal(err)
	}
	return out
}

func (h *harness) deliveries(t *testing.T) []row {
	t.Helper()
	return h.rows(t, `SELECT handler, event_id, code FROM event_deliveries ORDER BY handler`)
}

func (h *harness) handled(t *testing.T) []row {
	t.Helper()
	return h.rows(t, `SELECT handler, event_id, '' FROM handled ORDER BY handler`)
}

func (h *harness) lines(t *testing.T, msg string) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(h.logs.bytes()))
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

func (h *harness) assertLines(t *testing.T, msg string, want ...map[string]any) {
	t.Helper()
	lines := h.lines(t, msg)
	if len(lines) != len(want) {
		t.Fatalf("%d %s lines, want %d: %v", len(lines), msg, len(want), lines)
	}
	for i, w := range want {
		for k, v := range w {
			if lines[i][k] != v {
				t.Fatalf("%s line %d %s = %v, want %v (%v)", msg, i, k, lines[i][k], v, lines[i])
			}
		}
	}
}

type fakeMsg struct {
	subject string
	header  nats.Header
	data    []byte
	meta    *jetstream.MsgMetadata
	metaErr error
	verdict string
	delay   time.Duration
	reason  string
	ackErr  error
}

func fromMsg(msg jetstream.Msg, delivered uint64) *fakeMsg {
	return &fakeMsg{
		subject: msg.Subject(), header: msg.Headers(), data: msg.Data(),
		meta: &jetstream.MsgMetadata{NumDelivered: delivered},
	}
}

func (m *fakeMsg) Metadata() (*jetstream.MsgMetadata, error) { return m.meta, m.metaErr }
func (m *fakeMsg) Data() []byte                              { return m.data }
func (m *fakeMsg) Headers() nats.Header                      { return m.header }
func (m *fakeMsg) Subject() string                           { return m.subject }
func (m *fakeMsg) Reply() string                             { return "" }
func (m *fakeMsg) Ack() error                                { return m.respond("ack") }
func (m *fakeMsg) DoubleAck(context.Context) error           { return m.respond("ack") }
func (m *fakeMsg) Nak() error                                { return m.respond("nak") }
func (m *fakeMsg) NakWithDelay(d time.Duration) error        { m.delay = d; return m.respond("nak") }
func (m *fakeMsg) InProgress() error                         { return nil }
func (m *fakeMsg) Term() error                               { return m.respond("term") }
func (m *fakeMsg) TermWithReason(reason string) error        { m.reason = reason; return m.respond("term") }

func (m *fakeMsg) respond(verdict string) error {
	m.verdict = verdict
	return m.ackErr
}

func TestDispatch_verdictFollowsTheErrorCode(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var calls atomic.Int32
	failing := bus.Handle("notify.push", func(context.Context, db.Tx, events.SystemPinged) error {
		if calls.Add(1) == 1 {
			return errs.New(errs.CodeUpstreamUnavailable, "apns.Send")
		}
		return errs.New(errs.CodeInvalidInput, "notify.Render")
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{failing}})
	h.publishPing(t)
	first := fromMsg(h.fetch(t, h.consumer(t)), 1)
	second := fromMsg(first, 2)

	reg.Dispatch(h.ctx(t), durable, first)
	reg.Dispatch(h.ctx(t), durable, second)
	if first.verdict != "nak" || second.verdict != "term" {
		t.Fatalf("verdicts = %q, %q; want nak for upstream_unavailable and term for invalid_input",
			first.verdict, second.verdict)
	}
	if d := h.deliveries(t); len(d) != 0 {
		t.Fatalf("event_deliveries = %v after two failures, want none", d)
	}
	h.assertLines(t, "bus.dispatched",
		map[string]any{"outcome": "nak", "code": "upstream_unavailable", "delivery": 1.0, "level": "WARN"},
		map[string]any{"outcome": "term", "code": "invalid_input", "alert": false, "delivery": 2.0, "level": "ERROR"},
	)
}

func TestDispatch_handlesTheEventOnceAndAcksARedelivery(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}})
	id := h.publishPing(t)
	cons := h.consumer(t)
	msg := h.fetch(t, cons)

	reg.Dispatch(h.ctx(t), durable, msg)
	<-time.After(3 * testkit.DefaultAckWait)
	info, err := cons.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	if info.NumAckPending != 0 || info.NumRedelivered != 0 {
		t.Fatalf("ack pending = %d, redelivered = %d after dispatch, want 0 and 0",
			info.NumAckPending, info.NumRedelivered)
	}
	want := []row{{"notify.push", id, "ok"}}
	if got := h.deliveries(t); !slices.Equal(got, want) {
		t.Fatalf("event_deliveries = %v, want %v", got, want)
	}

	redelivered := fromMsg(msg, 2)
	reg.Dispatch(h.ctx(t), durable, redelivered)
	if redelivered.verdict != "ack" {
		t.Fatalf("redelivery verdict = %q, want ack", redelivered.verdict)
	}
	if got := h.handled(t); len(got) != 1 || got[0].eventID != h.pingID(t, id) {
		t.Fatalf("handled rows after redelivery = %v, want one for the event", got)
	}
	h.assertLines(t, "bus.dispatched",
		map[string]any{
			"consumer": durable, "handler": "notify.push", "outcome": "ack", "code": "ok", "delivery": 1.0,
			"subject": h.bus.Conn.Subject("events.system.pinged"), "level": "INFO",
		},
		map[string]any{"handler": "notify.push", "outcome": "duplicate", "code": "ok", "delivery": 2.0},
	)
}

func (h *harness) pingID(t *testing.T, eventID uuid.UUID) uuid.UUID {
	t.Helper()
	var id uuid.UUID
	if err := h.pool.QueryRow(t.Context(), `SELECT aggregate_id FROM events WHERE id = $1`, eventID).
		Scan(&id); err != nil {
		t.Fatal(err)
	}
	return id
}

func TestDispatch_panicTermsWithCodePanicAndCommitsNothing(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	exploding := bus.Handle("notify.push", func(ctx context.Context, tx db.Tx, e events.SystemPinged) error {
		_, err := tx.Queries().Exec(ctx, `INSERT INTO handled (handler, event_id) VALUES ('notify.push', $1)`, e.PingID)
		if err != nil {
			return err
		}
		panic("boom")
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{exploding}})
	h.publishPing(t)
	msg := fromMsg(h.fetch(t, h.consumer(t)), 1)

	reg.Dispatch(h.ctx(t), durable, msg)
	if msg.verdict != "term" {
		t.Fatalf("verdict = %q, want term", msg.verdict)
	}
	if d, hd := h.deliveries(t), h.handled(t); len(d) != 0 || len(hd) != 0 {
		t.Fatalf("event_deliveries = %v, handled = %v after a panic, want neither", d, hd)
	}
	h.assertLines(t, "bus.dispatched",
		map[string]any{"outcome": "term", "code": "panic", "alert": true, "level": "ERROR"})
	if lines := h.lines(t, "bus.dispatched"); len(lines) != 1 || !contains(lines[0]["err"], "bus.Dispatch: panic") {
		t.Fatalf("lines = %v, want one carrying the panic error", lines)
	}
}

func contains(v any, want string) bool {
	s, ok := v.(string)
	return ok && bytes.Contains([]byte(s), []byte(want))
}

func TestDispatch_undecodableMessagesTermWithDecodeFailed(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}})
	id, payload := h.appendPing(t)
	subject := h.bus.Conn.Subject("events.system.pinged")
	header := func(msgID string) nats.Header { return nats.Header{jetstream.MsgIDHeader: []string{msgID}} }
	for _, tc := range []struct {
		name    string
		msg     *fakeMsg
		handler string
	}{
		{"not json", &fakeMsg{subject: subject, header: header(id.String()), data: []byte("nope")}, "notify.push"},
		{"unknown version", &fakeMsg{subject: subject, header: header(id.String()), data: []byte(`{"v":3}`)}, "notify.push"},
		{"missing msg id", &fakeMsg{subject: subject, header: nats.Header{}, data: payload}, "notify.push"},
		{"no handler for subject", &fakeMsg{subject: "events.widget.bumped", header: header(id.String()), data: payload}, ""},
	} {
		tc.msg.meta = &jetstream.MsgMetadata{NumDelivered: 1}
		reg.Dispatch(h.ctx(t), durable, tc.msg)
		if tc.msg.verdict != "term" {
			t.Fatalf("%s: verdict = %q, want term", tc.name, tc.msg.verdict)
		}
		lines := h.lines(t, "bus.dispatched")
		last := lines[len(lines)-1]
		if last["code"] != "decode_failed" || last["handler"] != tc.handler || last["alert"] != true {
			t.Fatalf("%s: line = %v, want decode_failed for handler %q with alert", tc.name, last, tc.handler)
		}
	}
	if d := h.deliveries(t); len(d) != 0 {
		t.Fatalf("event_deliveries = %v, want none", d)
	}
}

func TestDispatch_cancelledContextNaksSoTheMessageComesBack(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}})
	h.publishPing(t)
	msg := fromMsg(h.fetch(t, h.consumer(t)), 1)
	ctx, cancel := context.WithCancel(h.ctx(t))
	cancel()

	reg.Dispatch(ctx, durable, msg)
	if msg.verdict != "nak" {
		t.Fatalf("verdict = %q with the context cancelled, want nak", msg.verdict)
	}
	if d := h.deliveries(t); len(d) != 0 {
		t.Fatalf("event_deliveries = %v, want none", d)
	}
	h.assertLines(t, "bus.dispatched", map[string]any{"outcome": "nak", "code": "db_unavailable"})
}

func TestDispatch_respondFailureIsLoggedOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}})
	h.publishPing(t)
	msg := fromMsg(h.fetch(t, h.consumer(t)), 1)
	msg.ackErr = jetstream.ErrMsgAlreadyAckd
	msg.metaErr = jetstream.ErrNotJSMessage

	reg.Dispatch(h.ctx(t), durable, msg)
	h.assertLines(t, "bus.respond_failed",
		map[string]any{"consumer": durable, "verdict": "ack", "err": "nats: message was already acknowledged"})
	h.assertLines(t, "bus.dispatched", map[string]any{"outcome": "ack", "delivery": 0.0})
}

func TestDispatch_unregisteredConsumerPanics(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	reg := h.registry(t)
	defer func() {
		if got := recover(); got != "bus: Dispatch for unregistered consumer ghost" {
			t.Fatalf("panic = %v", got)
		}
	}()
	reg.Dispatch(h.ctx(t), "ghost", &fakeMsg{})
}

func TestNewRegistry_panicsOnDuplicateDurableOrHandlerName(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	for _, tc := range []struct {
		name, want string
		consumers  []bus.Consumer
	}{
		{"durable", "bus: consumer notify registered twice", []bus.Consumer{
			{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}},
			{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.mail")}},
		}},
		{"handler", "bus: handler notify.push registered twice", []bus.Consumer{
			{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}},
			{Durable: "feed", Handlers: []bus.HandlerSpec{h.recorder("notify.push")}},
		}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			defer func() {
				if got := recover(); got != tc.want {
					t.Fatalf("panic = %v, want %q", got, tc.want)
				}
			}()
			h.registry(t, tc.consumers...)
		})
	}
}

func TestDispatch_eventIDMissingFromTheEventsTableTerms(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}})
	_, payload := h.appendPing(t)
	msg := &fakeMsg{
		subject: h.bus.Conn.Subject("events.system.pinged"),
		header:  nats.Header{jetstream.MsgIDHeader: []string{h.ids.NewV7().String()}},
		data:    payload,
		meta:    &jetstream.MsgMetadata{NumDelivered: 1},
	}

	reg.Dispatch(h.ctx(t), durable, msg)
	if msg.verdict != "term" {
		t.Fatalf("verdict = %q for an event id with no events row, want term", msg.verdict)
	}
	h.assertLines(t, "bus.dispatched", map[string]any{"outcome": "term", "code": "internal", "alert": true})
	if d, hd := h.deliveries(t), h.handled(t); len(d) != 0 || len(hd) != 0 {
		t.Fatalf("event_deliveries = %v, handled = %v, want neither", d, hd)
	}
}
