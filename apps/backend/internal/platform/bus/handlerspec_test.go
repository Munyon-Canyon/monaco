package bus_test

import (
	"context"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func TestHandlerSpec_beforeRunsWithTheEventIDAheadOfTheHandler(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var order []string
	spec := h.recorder("notify.push").Before(func(ctx context.Context, e events.Event) {
		order = append(order, "before "+observability.EventIDFrom(ctx)+" "+string(e.Type()))
	})
	if spec.Type() != events.TypeSystemPinged || spec.Name != "notify.push" {
		t.Fatalf("Before changed the spec: type %s, name %s", spec.Type(), spec.Name)
	}
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{spec}})
	id, payload := h.appendPing(t)
	msg := &fakeMsg{
		subject: h.bus.Conn.Subject(events.TypeSystemPinged.Subject()),
		header:  nats.Header{jetstream.MsgIDHeader: []string{id.String()}},
		data:    payload, meta: &jetstream.MsgMetadata{NumDelivered: 1},
	}

	reg.Dispatch(h.ctx(t), durable, msg)
	want := "before " + id.String() + " system.pinged"
	if msg.verdict != "ack" || len(order) != 1 || order[0] != want || len(h.handled(t)) != 1 {
		t.Fatalf("verdict %q, before calls %v (want %q), handled %d rows", msg.verdict, order, want, len(h.handled(t)))
	}
}
