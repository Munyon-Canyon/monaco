package bus_test

import (
	"bytes"
	"context"
	"encoding/json"
	"reflect"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func await(t *testing.T, what string, done <-chan struct{}) {
	t.Helper()
	select {
	case <-done:
	case <-time.After(waitLong):
		t.Fatalf("timed out waiting for %s", what)
	}
}

func startRegistry(ctx context.Context, t *testing.T, reg *bus.Registry) {
	t.Helper()
	stop, err := reg.Start(ctx)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(stop)
}

func quick() []time.Duration { return []time.Duration{10 * time.Millisecond} }

type deadLetter struct {
	Consumer string          `json:"consumer"`
	Handler  string          `json:"handler"`
	Subject  string          `json:"subject"`
	MsgID    string          `json:"msg_id"`
	Delivery uint64          `json:"delivery"`
	Code     string          `json:"code"`
	Error    string          `json:"error"`
	Data     json.RawMessage `json:"data"`
	Advisory json.RawMessage `json:"advisory"`
	Alert    bool            `json:"alert"`
	subject  string
}

func compact(t *testing.T, raw []byte) json.RawMessage {
	t.Helper()
	var buf bytes.Buffer
	if err := json.Compact(&buf, raw); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func (h *harness) deadLetters(t *testing.T) []deadLetter {
	t.Helper()
	s, err := h.bus.JS.Stream(t.Context(), h.bus.DeadLetter)
	if err != nil {
		t.Fatal(err)
	}
	info, err := s.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	var out []deadLetter
	for seq := info.State.FirstSeq; seq <= info.State.LastSeq && info.State.Msgs > 0; seq++ {
		raw, err := s.GetMsg(t.Context(), seq)
		if err != nil {
			t.Fatal(err)
		}
		var dl deadLetter
		if err := json.Unmarshal(raw.Data, &dl); err != nil {
			t.Fatal(err)
		}
		dl.subject = raw.Subject
		if dl.Data != nil {
			dl.Data = compact(t, dl.Data)
		}
		out = append(out, dl)
	}
	return out
}

func (h *harness) assertNoRedelivery(t *testing.T) uint64 {
	t.Helper()
	cons, err := h.bus.JS.Consumer(t.Context(), h.bus.Events, durable)
	if err != nil {
		t.Fatal(err)
	}
	return testkit.AssertNoRedelivery(t, cons).Delivered.Consumer
}

func (h *harness) assertNoDeliverySince(t *testing.T, delivered uint64) {
	t.Helper()
	const maxDeliver uint64 = 10
	ci := h.consumerInfo(t)
	if ci.Config.MaxDeliver != int(maxDeliver) {
		t.Fatalf("consumer MaxDeliver = %d, want %d", ci.Config.MaxDeliver, maxDeliver)
	}
	if ci.Delivered.Consumer != delivered || delivered >= maxDeliver {
		t.Fatalf("delivered %d times by the term, %d after it; want none after it, and fewer than MaxDeliver",
			delivered, ci.Delivered.Consumer-delivered)
	}
}

func waitCalls(t *testing.T, calls *atomic.Uint64, want uint64) {
	t.Helper()
	testkit.Eventually(t, func() bool { return calls.Load() == want }, waitLong)
}

func (h *harness) waitDeadLetters(t *testing.T, n int) []deadLetter {
	t.Helper()
	var got []deadLetter
	testkit.Eventually(t, func() bool {
		got = h.deadLetters(t)
		return len(got) >= n
	}, waitLong)
	return got
}

func TestDispatch_retryableErrorNaksAndTheRedeliverySucceeds(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var calls atomic.Int32
	done := make(chan struct{})
	flaky := bus.Handle("notify.push", func(ctx context.Context, tx db.Tx, e events.SystemPinged, _ time.Time) error {
		if calls.Add(1) == 1 {
			return errs.New(errs.CodeUpstreamUnavailable, "apns.Send")
		}
		_, err := tx.Queries().Exec(ctx, `INSERT INTO handled (handler, event_id) VALUES ('notify.push', $1)`, e.PingID)
		close(done)
		return err
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{flaky}, NakDelays: quick()})
	startRegistry(h.ctx(t), t, reg)

	id := h.publishPing(t)
	await(t, "second delivery", done)
	h.assertNoRedelivery(t)
	if got := h.deliveries(t); !slices.Equal(got, []row{{"notify.push", id, "ok"}}) {
		t.Fatalf("event_deliveries = %v, want the one ok row", got)
	}
	if n := calls.Load(); n != 2 {
		t.Fatalf("handler ran %d times, want 2", n)
	}
	if dl := h.deadLetters(t); len(dl) != 0 {
		t.Fatalf("DEADLETTER = %v after a nak, want empty", dl)
	}
	h.assertLinesThenDuplicates(t,
		map[string]any{"outcome": "nak", "code": "upstream_unavailable", "delivery": 1.0, "level": "WARN"},
		map[string]any{"outcome": "ack", "code": "ok", "delivery": 2.0},
	)
}

func (h *harness) assertLinesThenDuplicates(t *testing.T, want ...map[string]any) {
	t.Helper()
	lines := h.lines(t, "bus.dispatched")
	if len(lines) < len(want) {
		t.Fatalf("bus.dispatched = %v, want at least %d lines", lines, len(want))
	}
	for i, w := range want {
		for k, v := range w {
			if lines[i][k] != v {
				t.Fatalf("bus.dispatched line %d %s = %v, want %v (%v)", i, k, lines[i][k], v, lines[i])
			}
		}
	}
	for _, late := range lines[len(want):] {
		if late["outcome"] != "duplicate" {
			t.Fatalf("a delivery past the ack was %v, want duplicate", late)
		}
	}
}

func TestDispatch_nonRetryableErrorTermsIntoDeadLetterAndIsNotRedelivered(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var calls atomic.Uint64
	seen := make(chan struct{}, 1)
	rejecting := bus.Handle("notify.push", func(context.Context, db.Tx, events.SystemPinged, time.Time) error {
		calls.Add(1)
		select {
		case seen <- struct{}{}:
		default:
		}
		return errs.New(errs.CodeInvalidInput, "notify.Render")
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{rejecting}})
	startRegistry(h.ctx(t), t, reg)

	id, payload := h.appendPing(t)
	h.publish(t, id, payload)
	await(t, "first delivery", seen)
	got := h.waitDeadLetters(t, 1)
	delivered := h.assertNoRedelivery(t)
	h.assertNoDeliverySince(t, delivered)
	waitCalls(t, &calls, delivered)
	if got := h.deliveries(t); len(got) != 0 {
		t.Fatalf("event_deliveries = %v after term, want none", got)
	}
	want := deadLetter{
		Consumer: durable, Handler: "notify.push", Subject: h.bus.Conn.Subject("events.system.pinged"),
		MsgID: id.String(), Delivery: 1, Code: "invalid_input", Error: "notify.Render: invalid_input",
		Data: compact(t, payload), subject: h.bus.Conn.Subject("deadletter.notify"),
	}
	if len(got) != 1 || !reflect.DeepEqual(got[0], want) {
		t.Fatalf("DEADLETTER = %+v, want %+v", got, want)
	}
	if lines := h.lines(t, "bus.dispatched"); lines[0]["outcome"] != "term" || lines[0]["code"] != "invalid_input" ||
		lines[0]["alert"] != false || lines[0]["level"] != "ERROR" || lines[0]["delivery"] != 1.0 {
		t.Fatalf("bus.dispatched = %v, want term invalid_input on delivery 1", lines)
	}
}

func TestDispatch_twoHandlersInOneConsumerDedupeIndependently(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var pushCalls, mailCalls atomic.Int32
	done := make(chan struct{})
	push := bus.Handle("notify.push", func(ctx context.Context, tx db.Tx, e events.SystemPinged, _ time.Time) error {
		pushCalls.Add(1)
		_, err := tx.Queries().Exec(ctx, `INSERT INTO handled (handler, event_id) VALUES ('notify.push', $1)`, e.PingID)
		return err
	})
	mail := bus.Handle("notify.mail", func(ctx context.Context, tx db.Tx, e events.SystemPinged, _ time.Time) error {
		if mailCalls.Add(1) == 1 {
			return errs.New(errs.CodeUpstreamTimeout, "mail.Send")
		}
		_, err := tx.Queries().Exec(ctx, `INSERT INTO handled (handler, event_id) VALUES ('notify.mail', $1)`, e.PingID)
		close(done)
		return err
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{push, mail}, NakDelays: quick()})
	startRegistry(h.ctx(t), t, reg)

	id := h.publishPing(t)
	await(t, "mail's second delivery", done)
	h.assertNoRedelivery(t)
	want := []row{{"notify.mail", id, "ok"}, {"notify.push", id, "ok"}}
	if got := h.deliveries(t); !slices.Equal(got, want) {
		t.Fatalf("event_deliveries = %v, want %v", got, want)
	}
	if p, m := pushCalls.Load(), mailCalls.Load(); p != 1 || m != 2 {
		t.Fatalf("push ran %d times and mail %d, want 1 and 2", p, m)
	}
	if got := h.handled(t); len(got) != 2 {
		t.Fatalf("handled = %v, want one row per handler", got)
	}
}

func TestDispatch_nakDelayFollowsTheScheduleAndTermCarriesTheCode(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var calls atomic.Int32
	failing := bus.Handle("notify.push", func(context.Context, db.Tx, events.SystemPinged, time.Time) error {
		if calls.Add(1) <= 6 {
			return errs.New(errs.CodeUpstreamUnavailable, "apns.Send")
		}
		return errs.New(errs.CodeVersionConflict, "notify.Save")
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{failing}})
	h.publishPing(t)
	base := h.fetch(t, h.consumer(t))
	want := []time.Duration{
		time.Second,
		5 * time.Second,
		30 * time.Second,
		2 * time.Minute,
		10 * time.Minute,
		10 * time.Minute,
	}
	if !slices.Equal(bus.NakSchedule(), want[:5]) {
		t.Fatalf("NakSchedule() = %v, want %v", bus.NakSchedule(), want[:5])
	}
	for i, d := range want {
		msg := fromMsg(base, uint64(i+1))
		reg.Dispatch(h.ctx(t), durable, msg)
		if msg.verdict != "nak" || msg.delay != d {
			t.Fatalf("delivery %d: verdict %q delay %s, want nak after %s", i+1, msg.verdict, msg.delay, d)
		}
	}
	msg := fromMsg(base, 7)
	reg.Dispatch(h.ctx(t), durable, msg)
	if msg.verdict != "term" || msg.reason != "version_conflict" {
		t.Fatalf("verdict %q reason %q, want term with the code", msg.verdict, msg.reason)
	}
	if got := h.waitDeadLetters(t, 1); len(got) != 1 || got[0].Code != "version_conflict" || got[0].Delivery != 7 {
		t.Fatalf("DEADLETTER = %+v, want one version_conflict letter from delivery 7", got)
	}
}

func (h *harness) badMsg(t *testing.T, header nats.Header) *fakeMsg {
	t.Helper()
	id, _ := h.appendPing(t)
	header.Set(jetstream.MsgIDHeader, id.String())
	return &fakeMsg{
		subject: h.bus.Conn.Subject("events.system.pinged"), header: header,
		data: []byte("nope"), meta: &jetstream.MsgMetadata{NumDelivered: 1},
	}
}

func (h *harness) breakDeadLetter(t *testing.T) (restore func()) {
	t.Helper()
	ctx := h.ctx(t)
	if err := h.bus.JS.DeleteStream(ctx, h.bus.DeadLetter); err != nil {
		t.Fatal(err)
	}
	var once sync.Once
	restore = func() {
		once.Do(func() {
			if _, err := h.bus.Conn.Apply(context.WithoutCancel(ctx)); err != nil {
				t.Error(err)
			}
		})
	}
	t.Cleanup(restore)
	return restore
}

func TestDispatch_deadLetterPublishFailureKeepsTheSourceUntilTheLetterIsStored(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}})
	msg := h.badMsg(t, nats.Header{})
	id := msg.header.Get(jetstream.MsgIDHeader)
	restore := h.breakDeadLetter(t)

	reg.Dispatch(h.ctx(t), durable, msg)
	if msg.verdict != "nak" {
		t.Fatalf("verdict %q with the dead letter unavailable, want nak so the source is kept", msg.verdict)
	}
	h.assertLines(
		t,
		"bus.deadletter_dropped",
		map[string]any{"consumer": durable, "msg_id": id, "level": "ERROR"},
	)

	restore()
	again := fromMsg(msg, 2)
	reg.Dispatch(h.ctx(t), durable, again)
	if again.verdict != "term" || again.reason != "decode_failed" {
		t.Fatalf("verdict %q reason %q, want term decode_failed once the letter is stored", again.verdict, again.reason)
	}
	reg.Dispatch(h.ctx(t), durable, fromMsg(msg, 3))
	got := h.deadLetters(t)
	if len(got) != 1 || got[0].MsgID != id || got[0].Code != "decode_failed" || got[0].Delivery != 2 {
		t.Fatalf("DEADLETTER = %+v, want exactly one decode_failed letter for %s from delivery 2", got, id)
	}
}

func terminalOwn(h *harness, runs *atomic.Int32) bus.HandlerSpec {
	return bus.HandleOwn("trading.engine", func(ctx context.Context, d bus.Delivery, _ events.SystemPinged) error {
		if runs.Add(1) > 1 {
			return nil
		}
		err := h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			_, err := d.RecordAs(ctx, tx, string(errs.CodeInvalidInput))
			return err
		})
		if err != nil {
			return err
		}
		return errs.New(errs.CodeInvalidInput, "trading.Reject")
	})
}

func TestDispatch_redeliveryAfterARecordedTerminalDeliveryWritesTheMissingLetter(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var runs atomic.Int32
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{terminalOwn(h, &runs)}})
	id, msg := h.pingMsg(t)
	restore := h.breakDeadLetter(t)

	reg.Dispatch(h.ctx(t), durable, msg)
	if msg.verdict != "nak" {
		t.Fatalf("verdict %q with the letter unavailable, want nak", msg.verdict)
	}
	want := []row{
		{"trading.engine", id, "invalid_input"},
		{"trading.engine/deadletter", id, "invalid_input"},
	}
	if got := h.deliveries(t); !slices.Equal(got, want) {
		t.Fatalf("event_deliveries = %v, want %v: the held letter must survive the nak", got, want)
	}

	restore()
	again := fromMsg(msg, 2)
	reg.Dispatch(h.ctx(t), durable, again)
	if again.verdict != "term" || again.reason != "invalid_input" || runs.Load() != 1 {
		t.Fatalf("verdict %q reason %q after %d runs, want term invalid_input without rerunning the handler",
			again.verdict, again.reason, runs.Load())
	}
	got := h.deadLetters(t)
	if len(got) != 1 || got[0].Handler != "trading.engine" || got[0].Code != "invalid_input" || got[0].Delivery != 2 {
		t.Fatalf("DEADLETTER = %+v, want one invalid_input letter for trading.engine", got)
	}
}

func TestDispatch_redeliveryOfAnOwnHandlerWithNoHeldLetterRunsIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var runs atomic.Int32
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{terminalOwn(h, &runs)}})
	_, msg := h.pingMsg(t)
	msg.meta.NumDelivered = 2

	reg.Dispatch(h.ctx(t), durable, msg)
	if msg.verdict != "term" || runs.Load() != 1 || len(h.deadLetters(t)) != 1 {
		t.Fatalf("verdict %q after %d runs, want one run and a term into a letter", msg.verdict, runs.Load())
	}
}

func TestDispatch_aFailedHoldStillNaksTheSource(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var runs atomic.Int32
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{terminalOwn(h, &runs)}})
	_, msg := h.pingMsg(t)
	msg.header = nats.Header{jetstream.MsgIDHeader: []string{h.ids.NewV7().String()}}
	msg.meta.NumDelivered = bus.MaxDeliver
	h.breakDeadLetter(t)

	reg.Dispatch(h.ctx(t), durable, msg)
	if msg.verdict != "nak" {
		t.Fatalf("verdict %q, want nak even when the held letter cannot be stored", msg.verdict)
	}
	h.assertLines(t, "bus.deadletter_dropped",
		map[string]any{"consumer": durable, "level": "ERROR"},
		map[string]any{"handler": "trading.engine", "phase": "hold", "level": "ERROR"},
	)
}

func TestDispatch_aHeldLetterReadFailureNaksInsteadOfAcking(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var runs atomic.Int32
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{terminalOwn(h, &runs)}})
	_, msg := h.pingMsg(t)
	msg.meta.NumDelivered = 2
	h.pool.Close()

	reg.Dispatch(h.ctx(t), durable, msg)
	if msg.verdict != "nak" || runs.Load() != 0 {
		t.Fatalf("verdict %q after %d runs, want nak without running the handler", msg.verdict, runs.Load())
	}
}

func TestDispatch_aRedrivenSourceIsKeptUntilTheResolvedLetterIsStored(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}})
	id, msg := h.pingMsg(t)
	msg.header.Set(bus.RedrivenHeader, durable)
	restore := h.breakDeadLetter(t)

	reg.Dispatch(h.ctx(t), durable, msg)
	if msg.verdict != "nak" {
		t.Fatalf("verdict %q with the resolved letter unavailable, want nak", msg.verdict)
	}

	restore()
	again := fromMsg(msg, 2)
	reg.Dispatch(h.ctx(t), durable, again)
	got := h.deadLetters(t)
	if again.verdict != "ack" || len(got) != 1 || got[0].Code != bus.ResolvedCode || got[0].MsgID != id.String() {
		t.Fatalf("verdict %q, DEADLETTER %+v; want ack after one resolved letter for %s", again.verdict, got, id)
	}
}

func TestRegistry_aMaxDeliveriesAdvisoryIsRetriedUntilTheLetterIsStored(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	always := bus.Handle("notify.push", func(context.Context, db.Tx, events.SystemPinged, time.Time) error {
		return errs.New(errs.CodeUpstreamUnavailable, "apns.Send")
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{always}, NakDelays: quick()})
	restore := h.breakDeadLetter(t)
	startRegistry(h.ctx(t), t, reg)

	h.publishPing(t)
	testkit.Eventually(t, func() bool { return len(h.lines(t, "bus.deadletter_dropped")) > 0 }, waitLong)
	restore()
	testkit.Eventually(t, func() bool {
		h.clock.Advance(5 * time.Second)
		return len(h.deadLetters(t)) > 0
	}, waitLong)
	got := h.deadLetters(t)
	if len(got) != 1 || got[0].Advisory == nil || got[0].Consumer != durable {
		t.Fatalf("DEADLETTER = %+v, want exactly one advisory letter once the stream is back", got)
	}
}

func TestRegistry_aStoppedAdvisoryRetryGivesUp(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	always := bus.Handle("notify.push", func(context.Context, db.Tx, events.SystemPinged, time.Time) error {
		return errs.New(errs.CodeUpstreamUnavailable, "apns.Send")
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{always}, NakDelays: quick()})
	restore := h.breakDeadLetter(t)
	ctx, cancel := context.WithCancel(h.ctx(t))
	defer cancel()
	startRegistry(ctx, t, reg)

	h.publishPing(t)
	testkit.Eventually(t, func() bool { return len(h.lines(t, "bus.deadletter_dropped")) > 0 }, waitLong)
	cancel()
	restore()
	time.Sleep(200 * time.Millisecond)
	h.clock.Advance(5 * time.Second)
	time.Sleep(200 * time.Millisecond)
	if got := h.deadLetters(t); len(got) != 0 {
		t.Fatalf("DEADLETTER = %+v after the registry stopped, want none", got)
	}
}

func TestRegistry_everyMaxDeliveriesAdvisoryLandsInDeadLetter(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	always := bus.Handle("notify.push", func(context.Context, db.Tx, events.SystemPinged, time.Time) error {
		return errs.New(errs.CodeUpstreamUnavailable, "apns.Send")
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{always}, NakDelays: quick()})
	startRegistry(h.ctx(t), t, reg)

	h.publishPing(t)
	h.publishPing(t)
	got := h.waitDeadLetters(t, 2)
	seqs := map[uint64]bool{}
	for _, letter := range got {
		var advisory struct {
			Type       string `json:"type"`
			Consumer   string `json:"consumer"`
			StreamSeq  uint64 `json:"stream_seq"`
			Deliveries int    `json:"deliveries"`
		}
		if err := json.Unmarshal(letter.Advisory, &advisory); err != nil {
			t.Fatal(err)
		}
		if letter.Consumer != durable || letter.subject != h.bus.Conn.Subject("deadletter.notify") ||
			advisory.Type != "io.nats.jetstream.advisory.v1.max_deliver" || advisory.Consumer != durable ||
			advisory.Deliveries != 10 {
			t.Fatalf("DEADLETTER letter = %+v advisory %+v, want a max_deliver advisory for notify", letter, advisory)
		}
		seqs[advisory.StreamSeq] = true
	}
	if len(got) != 2 || len(seqs) != 2 {
		t.Fatalf("DEADLETTER holds %d letters for %d stream sequences, want 2 and 2: %+v", len(got), len(seqs), got)
	}
}

func TestRegistry_aMessageBehindASlowHandlerIsDeliveredOnce(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	entered, release := make(chan struct{}), make(chan struct{})
	var calls atomic.Uint64
	var releaseOnce sync.Once
	unblock := func() { releaseOnce.Do(func() { close(release) }) }
	slowFirst := bus.Handle("notify.push",
		func(ctx context.Context, tx db.Tx, e events.SystemPinged, _ time.Time) error {
			if calls.Add(1) == 1 {
				close(entered)
				<-release
			}
			_, err := tx.Queries().Exec(ctx,
				`INSERT INTO handled (handler, event_id) VALUES ('notify.push', $1)`, e.PingID)
			return err
		})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{slowFirst}})
	startRegistry(h.ctx(t), t, reg)
	t.Cleanup(unblock)

	h.publishPing(t)
	await(t, "the slow first handler", entered)
	h.publishPing(t)
	testkit.Eventually(t, func() bool {
		ci := h.consumerInfo(t)
		return ci.NumPending+ci.Delivered.Consumer == 2
	}, waitLong)
	if ci := h.consumerInfo(t); ci.Delivered.Consumer != 1 || ci.NumPending != 1 {
		t.Fatalf("while the first handler runs the consumer delivered %d messages with %d pending, want 1 and 1",
			ci.Delivered.Consumer, ci.NumPending)
	}
	unblock()
	testkit.Eventually(t, func() bool { return len(h.handled(t)) == 2 }, waitLong)
	if dl := h.deadLetters(t); len(dl) != 0 {
		t.Fatalf("DEADLETTER = %v, want empty", dl)
	}
}

func TestRegistry_startFailsOnAClosedConnection(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}})
	h.bus.Conn.Close(t.Context())
	_, err := reg.Start(h.ctx(t))
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || !strings.Contains(err.Error(), "bus.Registry.Start") {
		t.Fatalf("Start on a closed connection = %v, want upstream_unavailable from Start", err)
	}
}

func TestRegistry_startCreatesTheDurableWithTheRFCConfig(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.ctx(t)
	startRegistry(
		ctx,
		t,
		h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}}),
	)
	prod, err := bus.NewRegistry(h.bus.Conn, h.uow, h.clock,
		[]bus.Consumer{{Durable: "feed", Handlers: []bus.HandlerSpec{h.recorder("feed.fanout")}}})
	if err != nil {
		t.Fatal(err)
	}
	startRegistry(ctx, t, prod)
	for _, tc := range []struct {
		durable string
		ackWait time.Duration
		backOff []time.Duration
	}{
		{durable, testkit.DefaultAckWait, nil},
		{"feed", 30 * time.Second, []time.Duration{30 * time.Second, time.Minute, 5 * time.Minute, 15 * time.Minute}},
	} {
		info, err := h.bus.JS.Consumer(ctx, h.bus.Events, tc.durable)
		if err != nil {
			t.Fatal(err)
		}
		cfg := info.CachedInfo().Config
		if cfg.MaxDeliver != 10 || cfg.MaxAckPending != 64 || cfg.DeliverPolicy != jetstream.DeliverNewPolicy ||
			cfg.AckPolicy != jetstream.AckExplicitPolicy || cfg.AckWait != tc.ackWait ||
			!slices.Equal(cfg.BackOff, tc.backOff) ||
			!slices.Equal(cfg.FilterSubjects, []string{h.bus.Conn.Subject("events.system.pinged")}) {
			t.Fatalf("%s config = %+v", tc.durable, cfg)
		}
	}
}

func TestRegistry_warnsWhenTheRunningConsumerIsDeleted(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.ctx(t)
	startRegistry(
		ctx,
		t,
		h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}}),
	)
	testkit.Eventually(t, func() bool { return h.consumerInfo(t).NumWaiting > 0 }, waitLong)
	if err := h.bus.JS.DeleteConsumer(ctx, h.bus.Events, durable); err != nil {
		t.Fatal(err)
	}
	testkit.Eventually(t, func() bool { return len(h.lines(t, "bus.consume_error")) > 0 }, waitLong)
	if line := h.lines(t, "bus.consume_error")[0]; line["consumer"] != durable || line["level"] != "WARN" {
		t.Fatalf("line = %v, want a WARN naming %s", line, durable)
	}
}

func TestRegistry_startFailsWithoutTheStream(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}})
	ctx := h.ctx(t)
	if err := h.bus.JS.DeleteStream(ctx, h.bus.Events); err != nil {
		t.Fatal(err)
	}
	_, err := reg.Start(ctx)
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("Start without the stream = %v, want upstream_unavailable", err)
	}
	if _, err := h.bus.Conn.Apply(ctx); err != nil {
		t.Fatal(err)
	}
}

func TestRegistry_aMaxDeliveriesLetterCarriesTheEventPayloadSoItOutlivesEvents(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	always := bus.Handle("notify.push", func(context.Context, db.Tx, events.SystemPinged, time.Time) error {
		return errs.New(errs.CodeUpstreamUnavailable, "apns.Send")
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{always}, NakDelays: quick()})
	startRegistry(h.ctx(t), t, reg)
	id, payload := h.appendPing(t)
	h.publish(t, id, payload)

	letter := h.waitDeadLetters(t, 1)[0]
	if letter.Subject != h.bus.Conn.Subject(events.TypeSystemPinged.Subject()) ||
		!bytes.Equal(letter.Data, compact(t, payload)) {
		t.Fatalf("letter = %+v, want the ping's subject and payload %s", letter, payload)
	}
	if err := h.bus.JS.DeleteStream(t.Context(), h.bus.Events); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = h.bus.Conn.Apply(context.WithoutCancel(t.Context())) })
}
