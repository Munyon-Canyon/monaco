package bus_test

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func (h *harness) pingMsg(t *testing.T) (uuid.UUID, *fakeMsg) {
	t.Helper()
	id, payload := h.appendPing(t)
	return id, &fakeMsg{
		subject: h.bus.Conn.Subject(events.TypeSystemPinged.Subject()),
		header:  nats.Header{jetstream.MsgIDHeader: []string{id.String()}},
		data:    payload, meta: &jetstream.MsgMetadata{NumDelivered: 1},
	}
}

func TestHandleOwn_runsOutsideATransactionAndRecordsOnlyWhatTheHandlerRecords(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var seen []bus.Delivery
	own := bus.HandleOwn("trading.engine", func(ctx context.Context, d bus.Delivery, _ events.SystemPinged) error {
		seen = append(seen, d)
		if len(seen) == 1 {
			return nil
		}
		return h.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
			first, err := d.Record(ctx, tx)
			if err != nil {
				return err
			}
			second, err := d.Record(ctx, tx)
			if !first || second {
				return fmt.Errorf("Record inserted %v then %v, want true then false", first, second)
			}
			return err
		})
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{own}})
	id, msg := h.pingMsg(t)

	reg.Dispatch(h.ctx(t), durable, msg)
	if msg.verdict != "ack" || len(h.deliveries(t)) != 0 {
		t.Fatalf("first run: verdict %q, deliveries %v; want ack and no row, the handler recorded none",
			msg.verdict, h.deliveries(t))
	}
	reg.Dispatch(h.ctx(t), durable, fromMsg(msg, 2))
	want := []row{{handler: "trading.engine", eventID: id, code: "ok"}}
	if got := h.deliveries(t); len(got) != 1 || got[0] != want[0] {
		t.Fatalf("deliveries = %v, want %v: Record writes one row however often it runs", got, want)
	}
	last := seen[1]
	if last.Handler != "trading.engine" || last.EventID != ids.EventIDFrom(id) || !last.At.Equal(h.clock.Now()) {
		t.Fatalf("delivery = %+v, want the handler name, the event id and the clock's time", last)
	}
}

func TestHandleOwn_retryableErrorNaksAndAPanicTerms(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	calls := 0
	own := bus.HandleOwn("trading.engine", func(context.Context, bus.Delivery, events.SystemPinged) error {
		calls++
		if calls == 1 {
			return errs.New(errs.CodeJupiterUnavailable, "jupiter.Quote")
		}
		panic("gremlins")
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{own}})
	_, msg := h.pingMsg(t)
	again := fromMsg(msg, 2)

	reg.Dispatch(h.ctx(t), durable, msg)
	reg.Dispatch(h.ctx(t), durable, again)
	if msg.verdict != "nak" || again.verdict != "term" || again.reason != string(errs.CodePanic) {
		t.Fatalf("verdicts %q, %q (%s); want nak for jupiter_unavailable and term for a panic",
			msg.verdict, again.verdict, again.reason)
	}
}

func TestHeartbeat_sendsInProgressInsideDispatchAndIsNilOutside(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if bus.Heartbeat(t.Context()) != nil {
		t.Fatal("Heartbeat outside Dispatch is not nil")
	}
	own := bus.HandleOwn("trading.engine", func(ctx context.Context, _ bus.Delivery, _ events.SystemPinged) error {
		beat := bus.Heartbeat(ctx)
		beat()
		beat()
		return nil
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{own}})
	_, msg := h.pingMsg(t)

	reg.Dispatch(h.ctx(t), durable, msg)
	if n := msg.inProgress.Load(); n != 2 || msg.verdict != "ack" {
		t.Fatalf("%d InProgress, verdict %q; want one per beat and an ack", n, msg.verdict)
	}
}

func TestHandleOwn_replayAndSeedingNeverRunIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	ran := false
	own := bus.HandleOwn("trading.engine", func(context.Context, bus.Delivery, events.SystemPinged) error {
		ran = true
		return nil
	})
	ev := events.SystemPinged{V: 1, PingID: h.ids.NewV7(), Note: "hi"}

	duplicate, err := bus.Deliver(h.ctx(t), h.uow, h.clock, own, ids.EventIDFrom(h.ids.NewV7()), ev)
	if err != nil || !duplicate {
		t.Fatalf("Deliver = %v, %v; want a duplicate with no error", duplicate, err)
	}
	if err := own.Apply(h.ctx(t), db.Tx{}, ev, time.Time{}); err != nil {
		t.Fatal(err)
	}
	if ran || !own.OwnIdempotency() || h.recorder("notify.push").OwnIdempotency() {
		t.Fatalf("ran %v, own %v; want the handler skipped and only HandleOwn marked own", ran, own.OwnIdempotency())
	}
}

func TestHandleOwn_beforeAndOnCommitWrapIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var order []string
	fail := true
	own := bus.HandleOwn("trading.engine", func(context.Context, bus.Delivery, events.SystemPinged) error {
		order = append(order, "handle")
		if fail {
			fail = false
			return errs.New(errs.CodeJupiterUnavailable, "jupiter.Quote")
		}
		return nil
	}).
		Before(func(context.Context, events.Event) { order = append(order, "before") }).
		OnCommit(func(context.Context, events.Event) { order = append(order, "committed") })
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{own}})
	_, msg := h.pingMsg(t)

	reg.Dispatch(h.ctx(t), durable, msg)
	reg.Dispatch(h.ctx(t), durable, fromMsg(msg, 2))
	want := []string{"before", "handle", "before", "handle", "committed"}
	if len(order) != len(want) {
		t.Fatalf("order = %v, want %v", order, want)
	}
	for i := range want {
		if order[i] != want[i] {
			t.Fatalf("order = %v, want %v: OnCommit runs only after a nil return", order, want)
		}
	}
}
