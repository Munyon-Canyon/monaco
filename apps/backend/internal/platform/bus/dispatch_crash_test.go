package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func dispatchRecovering(ctx context.Context, reg *bus.Registry, msg jetstream.Msg) (p any) {
	defer func() { p = recover() }()
	reg.Dispatch(ctx, durable, msg)
	return nil
}

func TestDispatch_aCrashPanicsThroughWithoutAVerdictOrACommit(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	crashing := bus.Handle(
		"notify.push",
		func(ctx context.Context, tx db.Tx, e events.SystemPinged, _ time.Time) error {
			_, err := tx.Queries().
				Exec(ctx, `INSERT INTO handled (handler, event_id) VALUES ('notify.push', $1)`, e.PingID)
			if err != nil {
				return err
			}
			panic(faultpoint.Crash{Name: faultpoint.AfterExecute})
		},
	)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{crashing}})
	h.publishPing(t)
	msg := fromMsg(h.fetch(t, h.consumer(t)), 1)

	if p := dispatchRecovering(h.ctx(t), reg, msg); p != (faultpoint.Crash{Name: faultpoint.AfterExecute}) {
		t.Fatalf("Dispatch recovered %v, want Crash{after-execute}", p)
	}
	if msg.verdict != "" {
		t.Fatalf("verdict = %q after a crash, want none so JetStream redelivers", msg.verdict)
	}
	if d, hd := h.deliveries(t), h.handled(t); len(d) != 0 || len(hd) != 0 {
		t.Fatalf("event_deliveries = %v, handled = %v after a crash, want neither", d, hd)
	}
}
