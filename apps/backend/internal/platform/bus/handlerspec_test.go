package bus_test

import (
	"context"
	"errors"
	"testing"

	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
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

func TestHandlerSpec_onCommitRunsOnlyAfterTheHandlerCommits(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var seen []int
	notify := func(ctx context.Context, _ events.Event) {
		var n int
		if err := h.pool.QueryRow(ctx, `SELECT count(*) FROM handled`).Scan(&n); err != nil {
			t.Error(err)
		}
		seen = append(seen, n)
	}
	refused := errs.New(errs.CodeNotFound, "test.refuse")
	for _, spec := range []bus.HandlerSpec{
		h.recorder("notify.push").OnCommit(notify),
		bus.Handle("notify.refuse", func(context.Context, db.Tx, events.SystemPinged) error { return refused }).
			OnCommit(notify),
	} {
		ev := events.SystemPinged{V: 1, PingID: h.ids.NewV7()}
		err := h.uow.Do(h.ctx(t), func(ctx context.Context, tx db.Tx) error { return spec.Apply(ctx, tx, ev) })
		if spec.Name == "notify.refuse" && !errors.Is(err, refused) {
			t.Fatalf("Apply(%s) = %v, want the refusal", spec.Name, err)
		}
	}
	if len(seen) != 1 || seen[0] != 1 {
		t.Fatalf("OnCommit saw %v handled rows, want one call that sees the committed row", seen)
	}
}
