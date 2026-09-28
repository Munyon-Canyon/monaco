package bus_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"
	"go.opentelemetry.io/otel/sdk/metric/metricdata"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const traceparent = "00-4bf92f3577b34da6a3ce929d0e0e4736-00f067aa0ba902b7-01"

func TestDispatch_extractsTheTraceAndAddsTheJoinKeys(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}})
	id, payload := h.appendPing(t)
	msg := &fakeMsg{
		subject: h.bus.Conn.Subject("events.system.pinged"),
		header:  nats.Header{jetstream.MsgIDHeader: []string{id.String()}, "Traceparent": []string{traceparent}},
		data:    payload, meta: &jetstream.MsgMetadata{NumDelivered: 3},
	}

	reg.Dispatch(h.ctx(t), durable, msg)
	h.assertLines(t, "bus.dispatched", map[string]any{
		"trace_id": "4bf92f3577b34da6a3ce929d0e0e4736", "span_id": "00f067aa0ba902b7", "event_id": id.String(),
		"consumer": durable, "delivery": 3.0, "outcome": "ack",
	})
}

func TestKeepAlive_sendsInProgressOnEveryTickUntilStopped(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var msg *fakeMsg
	var afterStop int32
	slow := bus.Handle("notify.push", func(ctx context.Context, _ db.Tx, _ events.SystemPinged) error {
		stop := bus.KeepAlive(ctx)
		for tick := int32(1); tick <= 3; tick++ {
			h.clock.Advance(10 * time.Second)
			deadline := time.After(waitLong)
			for msg.inProgress.Load() != tick {
				select {
				case <-deadline:
					return fmt.Errorf("tick %d sent %d InProgress", tick, msg.inProgress.Load())
				case <-time.After(time.Millisecond):
				}
			}
		}
		stop()
		h.clock.Advance(30 * time.Second)
		afterStop = msg.inProgress.Load()
		return nil
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{slow}})
	id, payload := h.appendPing(t)
	msg = &fakeMsg{
		subject: h.bus.Conn.Subject(events.TypeSystemPinged.Subject()),
		header:  nats.Header{jetstream.MsgIDHeader: []string{id.String()}},
		data:    payload, meta: &jetstream.MsgMetadata{NumDelivered: 1},
	}

	reg.Dispatch(h.ctx(t), durable, msg)
	if msg.verdict != "ack" || afterStop != 3 {
		t.Fatalf("verdict %q, %d InProgress after stop; want ack and 3, one per 10 s tick and none after stop\n%s",
			msg.verdict, afterStop, h.logs.bytes())
	}
}

func (h *harness) consumerInfo(t *testing.T) *jetstream.ConsumerInfo {
	t.Helper()
	cons, err := h.bus.JS.Consumer(t.Context(), h.bus.Events, durable)
	if err != nil {
		t.Fatal(err)
	}
	ci, err := cons.Info(t.Context())
	if err != nil {
		t.Fatal(err)
	}
	return ci
}

func TestKeepAlive_outsideDispatchIsANoOp(t *testing.T) {
	t.Parallel()
	bus.KeepAlive(t.Context())()
}

func (h *harness) histogramCounts(t *testing.T) map[string]uint64 {
	t.Helper()
	hist, ok := collect(t, h.reader, "monaco_bus_handler_duration_seconds").Data.(metricdata.Histogram[float64])
	if !ok {
		t.Fatal("handler duration histogram missing")
	}
	out := map[string]uint64{}
	for _, p := range hist.DataPoints {
		consumer, _ := p.Attributes.Value("consumer")
		subject, _ := p.Attributes.Value("subject")
		outcome, _ := p.Attributes.Value("outcome")
		out[consumer.AsString()+" "+subject.AsString()+" "+outcome.AsString()] = p.Count
	}
	return out
}

func (h *harness) gaugeByConsumer(t *testing.T, name string) map[string]int64 {
	t.Helper()
	g, ok := collect(t, h.reader, name).Data.(metricdata.Gauge[int64])
	if !ok {
		t.Fatalf("gauge %s missing", name)
	}
	out := map[string]int64{}
	for _, p := range g.DataPoints {
		consumer, _ := p.Attributes.Value("consumer")
		out[consumer.AsString()] = p.Value
	}
	return out
}

func TestDispatch_recordsHandlerDurationPerOutcome(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	rejecting := bus.Handle("notify.mail", func(context.Context, db.Tx, events.SystemPinged) error {
		return errs.New(errs.CodeInvalidInput, "mail.Render")
	})
	reg := h.registry(
		t,
		bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push"), rejecting}},
	)
	h.publishPing(t)
	msg := fromMsg(h.fetch(t, h.consumer(t)), 1)
	reg.Dispatch(h.ctx(t), durable, msg)
	reg.Dispatch(h.ctx(t), durable, fromMsg(msg, 2))

	want := map[string]uint64{
		"notify events.system.pinged ack":       1,
		"notify events.system.pinged duplicate": 1,
		"notify events.system.pinged term":      2,
	}
	got := h.histogramCounts(t)
	if len(got) != len(want) {
		t.Fatalf("histogram = %v, want %v", got, want)
	}
	for k, v := range want {
		if got[k] != v {
			t.Fatalf("histogram[%q] = %d, want %d (%v)", k, got[k], v, got)
		}
	}
}

func TestRegistry_gaugesReportPendingAckPendingAndDeadLetters(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	started := make(chan struct{}, 1)
	release := make(chan struct{})
	handler := bus.Handle("notify.push", func(ctx context.Context, _ db.Tx, _ events.SystemPinged) error {
		select {
		case started <- struct{}{}:
		default:
		}
		select {
		case <-release:
		case <-ctx.Done():
		}
		return errs.New(errs.CodeInvalidInput, "notify.Render")
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{handler}})
	startRegistry(h.ctx(t), t, reg)
	h.publishPing(t)
	await(t, "the handler to start", started)

	if got := h.gaugeByConsumer(t, "monaco_bus_consumer_ack_pending"); got[durable] != 1 {
		t.Fatalf("ack pending = %v, want 1 for %s", got, durable)
	}
	if got := h.gaugeByConsumer(t, "monaco_bus_consumer_pending"); got[durable] != 0 {
		t.Fatalf("pending = %v, want 0 for %s", got, durable)
	}
	h.waitDelivered(t, 3)
	close(release)
	h.waitDeadLetters(t, 1)
	if got := h.gaugeByConsumer(t, "monaco_dead_letters"); got[durable] != 1 {
		t.Fatalf("dead letters = %v, want 1 for %s", got, durable)
	}
	if got := h.gaugeByConsumer(t, "monaco_bus_consumer_ack_pending"); got[durable] != 0 {
		t.Fatalf("ack pending after term = %v, want 0", got)
	}
}

func (h *harness) waitDelivered(t *testing.T, n uint64) {
	t.Helper()
	deadline := time.After(waitLong)
	for h.consumerInfo(t).Delivered.Consumer < n {
		select {
		case <-deadline:
			t.Fatalf("the consumer never delivered %d times", n)
		case <-time.After(20 * time.Millisecond):
		}
	}
}

func TestRegistry_gaugesFailWhenTheConsumerOrTheStreamIsGone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ctx := h.ctx(t)
	startRegistry(
		ctx,
		t,
		h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}}),
	)
	if err := h.bus.JS.DeleteStream(ctx, h.bus.DeadLetter); err != nil {
		t.Fatal(err)
	}
	var rm metricdata.ResourceMetrics
	if err := h.reader.Collect(ctx, &rm); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("collect without DEADLETTER = %v, want upstream_unavailable", err)
	}
	if _, err := h.bus.Conn.Apply(ctx); err != nil {
		t.Fatal(err)
	}
	if err := h.bus.JS.DeleteConsumer(ctx, h.bus.Events, durable); err != nil {
		t.Fatal(err)
	}
	if err := h.reader.Collect(ctx, &rm); errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("collect without the consumer = %v, want upstream_unavailable", err)
	}
}

func TestNewRegistry_failsWhenAnInstrumentCannotBeCreated(t *testing.T) {
	t.Parallel()
	for _, fail := range []string{"histogram", "gauge", "callback"} {
		conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessWorker,
			bus.WithMeterProvider(failingMeters{fail: fail}), bus.WithNamespace("t_"+fail))
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { conn.Close(context.Background()) })
		reg, err := bus.NewRegistry(conn, nil, testkit.NewClock(time.Time{}), nil)
		if fail != "callback" {
			if errs.CodeOf(err) != errs.CodeInternal {
				t.Fatalf("%s: NewRegistry = %v, want internal", fail, err)
			}
			continue
		}
		if err != nil {
			t.Fatal(err)
		}
		if _, err := conn.Apply(t.Context()); err != nil {
			t.Fatal(err)
		}
		if _, err := reg.Start(t.Context()); errs.CodeOf(err) != errs.CodeInternal {
			t.Fatalf("Start with a failing callback = %v, want internal", err)
		}
	}
}
