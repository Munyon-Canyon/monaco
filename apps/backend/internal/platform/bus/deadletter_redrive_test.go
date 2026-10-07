package bus_test

import (
	"context"
	"slices"
	"sync/atomic"
	"testing"
	"time"

	"github.com/google/uuid"
	"github.com/nats-io/nats.go"
	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func (h *harness) assertMarker(t *testing.T, marker deadLetter, event string) {
	t.Helper()
	if marker.Code != "ok" || marker.Consumer != durable || marker.MsgID != event ||
		marker.subject != h.bus.Conn.Subject("deadletter.notify") {
		t.Fatalf("DEADLETTER message = %+v, want the Code ok marker for event %s", marker, event)
	}
}

func TestRedeliver_anAckedRedrivePublishesOneMarkerThatTheCLIReadersNeverSee(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var calls atomic.Int32
	flaky := bus.Handle("notify.push", func(ctx context.Context, tx db.Tx, e events.SystemPinged, _ time.Time) error {
		if calls.Add(1) == 1 {
			return errs.New(errs.CodeInvalidInput, "notify.Render")
		}
		_, err := tx.Queries().Exec(ctx, `INSERT INTO handled (handler, event_id) VALUES ('notify.push', $1)`, e.PingID)
		return err
	})
	startRegistry(h.ctx(t), t, h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{flaky}}))
	id := h.publishPing(t)
	h.waitDeadLetters(t, 1)
	letters, err := h.bus.Conn.DeadLetters(t.Context())
	if err != nil || len(letters) != 1 {
		t.Fatalf("DeadLetters = %+v, %v, want the one termed letter", letters, err)
	}

	if err := h.bus.Conn.Redeliver(t.Context(), letters[0], "redrive-1"); err != nil {
		t.Fatal(err)
	}
	h.assertMarker(t, h.waitDeadLetters(t, 2)[1], id.String())
	redriven, err := h.bus.Conn.EventAt(t.Context(), 2)
	if err != nil || redriven.Header.Get(bus.RedrivenHeader) != durable {
		t.Fatalf("EventAt(2) = %+v, %v, want the republished event marked %s", redriven, err, bus.RedrivenHeader)
	}
	if letters, err := h.bus.Conn.DeadLetters(t.Context()); err != nil || len(letters) != 1 {
		t.Fatalf("DeadLetters after the marker = %+v, %v, want the marker hidden", letters, err)
	}
	if err := h.bus.Conn.Redeliver(
		t.Context(),
		bus.DeadLetter{Code: "ok"},
		"redrive-2",
	); errs.CodeOf(
		err,
	) != errs.CodeNotFound {
		t.Fatalf("Redeliver of a marker = %v, want not_found", err)
	}
}

func TestRedeliver_aSecondRedriveOfAnAckedEventDedupesIntoItsOwnMarker(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var calls atomic.Int32
	flaky := bus.Handle("notify.push", func(ctx context.Context, tx db.Tx, e events.SystemPinged, _ time.Time) error {
		if calls.Add(1) == 1 {
			return errs.New(errs.CodeInvalidInput, "notify.Render")
		}
		_, err := tx.Queries().Exec(ctx, `INSERT INTO handled (handler, event_id) VALUES ('notify.push', $1)`, e.PingID)
		return err
	})
	startRegistry(h.ctx(t), t, h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{flaky}}))
	h.publishPing(t)
	h.waitDeadLetters(t, 1)
	all, err := h.bus.Conn.DeadLetters(t.Context())
	if err != nil || len(all) != 1 {
		t.Fatalf("DeadLetters = %+v, %v, want one", all, err)
	}
	for _, suffix := range []string{"redrive-1", "redrive-2"} {
		if err := h.bus.Conn.Redeliver(t.Context(), all[0], suffix); err != nil {
			t.Fatal(err)
		}
	}
	h.waitDeadLetters(t, 3)
	if got := h.handled(t); len(got) != 1 {
		t.Fatalf("handled rows = %v, want the handler to run once for two redrives", got)
	}
	if calls.Load() != 2 {
		t.Fatalf("handler calls = %d, want the second redrive to dedupe on the event id", calls.Load())
	}
}

func TestDispatch_onlyAnAckedRedrivenMessageWritesAMarkerAndATermedOneWritesAnOrdinaryLetter(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name      string
		redriven  bool
		failWith  errs.Code
		wantCodes []string
	}{
		{"acked redrive", true, "", []string{"ok"}},
		{"acked first delivery", false, "", nil},
		{"naked redrive", true, errs.CodeUpstreamUnavailable, nil},
		{"termed redrive", true, errs.CodeInvalidInput, []string{"invalid_input"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			h := newHarness(t)
			id := h.dispatchRedriven(t, tc.redriven, tc.failWith)
			h.assertLetters(t, id, tc.wantCodes)
		})
	}
}

func (h *harness) assertLetters(t *testing.T, event uuid.UUID, wantCodes []string) {
	t.Helper()
	letters := h.deadLetters(t)
	codes := make([]string, len(letters))
	for i, l := range letters {
		codes[i] = l.Code
		want := event.String() + "/redrive-1"
		if l.Code == "ok" {
			want = event.String()
		}
		if l.MsgID != want {
			t.Fatalf("%s letter MsgID = %q, want %q", l.Code, l.MsgID, want)
		}
	}
	if !slices.Equal(codes, wantCodes) {
		t.Fatalf("DEADLETTER codes = %v, want %v", codes, wantCodes)
	}
}

func (h *harness) dispatchRedriven(t *testing.T, redriven bool, failWith errs.Code) uuid.UUID {
	t.Helper()
	handler := bus.Handle("notify.push", func(context.Context, db.Tx, events.SystemPinged, time.Time) error {
		if failWith != "" {
			return errs.New(failWith, "notify.Render")
		}
		return nil
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{handler}})
	id, payload := h.appendPing(t)
	header := nats.Header{jetstream.MsgIDHeader: []string{id.String() + "/redrive-1"}}
	header.Set(bus.EventIDHeader, id.String())
	if redriven {
		header.Set(bus.RedrivenHeader, durable)
	}
	reg.Dispatch(h.ctx(t), durable, &fakeMsg{
		subject: h.bus.Conn.Subject("events.system.pinged"), header: header, data: payload,
		meta: &jetstream.MsgMetadata{NumDelivered: 1},
	})
	return id
}

func TestRedeliver_onlyTheRedrivenDurablesMarkerIsWrittenWhenAnotherDurableSharesTheSubject(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var calls atomic.Int32
	otherAcked := make(chan struct{})
	flaky := bus.Handle("notify.push", func(ctx context.Context, tx db.Tx, e events.SystemPinged, _ time.Time) error {
		if calls.Add(1) == 1 {
			return errs.New(errs.CodeInvalidInput, "notify.Render")
		}
		<-otherAcked
		_, err := tx.Queries().Exec(ctx, `INSERT INTO handled (handler, event_id) VALUES ('notify.push', $1)`, e.PingID)
		return err
	})
	startRegistry(h.ctx(t), t, h.registry(t,
		bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{flaky}},
		bus.Consumer{Durable: "other", Handlers: []bus.HandlerSpec{h.recorder("other.push")}},
	))
	id := h.publishPing(t)
	h.waitDeadLetters(t, 1)
	letters, err := h.bus.Conn.DeadLetters(t.Context())
	if err != nil || len(letters) != 1 || letters[0].Consumer != durable {
		t.Fatalf("DeadLetters = %+v, %v, want the one letter of %s", letters, err, durable)
	}
	if err := h.bus.Conn.Redeliver(t.Context(), letters[0], "redrive-1"); err != nil {
		t.Fatal(err)
	}
	other, err := h.bus.JS.Consumer(t.Context(), h.bus.Events, "other")
	if err != nil {
		t.Fatal(err)
	}
	testkit.Eventually(t, func() bool {
		info, err := other.Info(t.Context())
		return err == nil && info.AckFloor.Consumer >= 2
	}, waitLong)
	close(otherAcked)
	h.assertMarker(t, h.waitDeadLetters(t, 2)[1], id.String())
	testkit.Eventually(t, func() bool { return len(h.handled(t)) == 2 }, waitLong)
	if got := h.deadLetters(t); len(got) != 2 {
		t.Fatalf("DEADLETTER = %+v, want the letter and the marker of %s only, with none from other", got, durable)
	}
}
