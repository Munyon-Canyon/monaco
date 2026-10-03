//go:build faultpoints

package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

func TestDispatch_fetchedHandlerCrashBeforeCommitFetchesAgainAfterRedelivery(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var fetches int
	handler := bus.HandleFetched(
		"notify.push",
		func(context.Context, events.SystemPinged) (string, error) {
			fetches++
			return "value", nil
		},
		func(ctx context.Context, tx db.Tx, e events.SystemPinged, value string, _ time.Time) error {
			if value != "value" {
				t.Fatalf("value = %q, want value", value)
			}
			_, err := tx.Queries().Exec(
				ctx, `INSERT INTO handled (handler, event_id) VALUES ('notify.push', $1)`, e.PingID,
			)
			return err
		},
	)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{handler}})
	h.publishPing(t)
	msg := fromMsg(h.fetch(t, h.consumer(t)), 1)

	ctx := faultpoint.Armed(h.ctx(t), faultpoint.BeforeCommit)
	if p := dispatchRecovering(ctx, reg, msg); p != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
		t.Fatalf("Dispatch recovered %v, want Crash{before-commit}", p)
	}
	redelivered := fromMsg(msg, 2)
	reg.Dispatch(h.ctx(t), durable, redelivered)
	if fetches != 2 || msg.verdict != "" || redelivered.verdict != "ack" {
		t.Fatalf("fetches = %d, verdicts = %q, %q; want 2, empty, ack", fetches, msg.verdict, redelivered.verdict)
	}
	if d, handled := h.deliveries(t), h.handled(t); len(d) != 1 || len(handled) != 1 {
		t.Fatalf("event_deliveries = %v, handled = %v, want one each", d, handled)
	}
}

func TestDispatch_crashBeforeCommitLeavesNoDeliveryAndNoHandledRow(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{h.recorder("notify.push")}})
	h.publishPing(t)
	msg := fromMsg(h.fetch(t, h.consumer(t)), 1)

	ctx := faultpoint.Armed(h.ctx(t), faultpoint.BeforeCommit)
	if p := dispatchRecovering(ctx, reg, msg); p != (faultpoint.Crash{Name: faultpoint.BeforeCommit}) {
		t.Fatalf("Dispatch recovered %v, want Crash{before-commit}", p)
	}
	if d, hd := h.deliveries(t), h.handled(t); len(d) != 0 || len(hd) != 0 || msg.verdict != "" {
		t.Fatalf("event_deliveries = %v, handled = %v, verdict %q after a crash, want none", d, hd, msg.verdict)
	}

	redelivered := fromMsg(msg, 2)
	reg.Dispatch(h.ctx(t), durable, redelivered)
	if d, hd := h.deliveries(t), h.handled(t); len(d) != 1 || len(hd) != 1 || redelivered.verdict != "ack" {
		t.Fatalf("after redelivery event_deliveries = %v, handled = %v, verdict %q; want one each and ack",
			d, hd, redelivered.verdict)
	}
}
