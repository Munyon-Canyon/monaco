package bus_test

import (
	"context"
	"encoding/json"
	"errors"
	"slices"
	"testing"

	"github.com/nats-io/nats.go"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
)

const sinkDurable = "sink"

func (h *harness) putLetter(t *testing.T, letter bus.DeadLetter) {
	t.Helper()
	body, err := json.Marshal(letter)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := h.bus.JS.Publish(t.Context(), h.bus.Conn.Subject("deadletter."+letter.Consumer), body); err != nil {
		t.Fatal(err)
	}
}

func (h *harness) pull(t *testing.T, limit int, fn func(context.Context, bus.DeadLetter) error) (int, error) {
	t.Helper()
	return h.bus.Conn.PullDeadLetters(t.Context(), sinkDurable, limit, fn)
}

func TestPullDeadLetters_handsOverEveryLetterOnceWithItsStreamSequenceAndAcksIt(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.putLetter(t, bus.DeadLetter{Consumer: "notify", Code: "invalid_input", Error: "bad"})
	h.putLetter(t, bus.DeadLetter{Consumer: "analytics", Code: "ok", MsgID: "m-2"})

	var got []bus.DeadLetter
	n, err := h.pull(t, 100, func(_ context.Context, l bus.DeadLetter) error {
		got = append(got, l)
		return nil
	})
	if err != nil || n != 2 || len(got) != 2 {
		t.Fatalf("pull = %d, %v with %+v, want both letters", n, err, got)
	}
	if got[0].Seq != 1 || got[0].Consumer != "notify" || got[0].Code != "invalid_input" ||
		got[1].Seq != 2 || got[1].Code != "ok" || got[1].MsgID != "m-2" {
		t.Fatalf("letters = %+v, want sequences 1 and 2 in order, the marker included", got)
	}
	n, err = h.pull(t, 100, func(context.Context, bus.DeadLetter) error {
		t.Fatal("an acked letter was handed over again")
		return nil
	})
	if err != nil || n != 0 {
		t.Fatalf("second pull = %d, %v, want nothing left", n, err)
	}
}

func TestPullDeadLetters_aFailingHandlerNaksItsLetterAndTheOnesBehindItAndReturnsTheCountSoFar(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	h.putLetter(t, bus.DeadLetter{Consumer: "notify", MsgID: "first"})
	h.putLetter(t, bus.DeadLetter{Consumer: "notify", MsgID: "second"})
	h.putLetter(t, bus.DeadLetter{Consumer: "notify", MsgID: "third"})
	boom := errors.New("db down")

	n, err := h.pull(t, 100, func(_ context.Context, l bus.DeadLetter) error {
		if l.MsgID == "second" {
			return boom
		}
		return nil
	})
	if !errors.Is(err, boom) || n != 1 {
		t.Fatalf("pull = %d, %v, want 1 and the handler's error", n, err)
	}
	var again []string
	n, err = h.pull(t, 100, func(_ context.Context, l bus.DeadLetter) error {
		again = append(again, l.MsgID)
		return nil
	})
	if err != nil || n != 2 || !slices.Equal(again, []string{"second", "third"}) {
		t.Fatalf("retry pull = %d, %v with %v, want the failed letter and the one behind it", n, err, again)
	}
}

func TestPullDeadLetters_failsWhenTheStreamIsGone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	if err := h.bus.JS.DeleteStream(t.Context(), h.bus.DeadLetter); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = h.bus.Conn.Apply(context.WithoutCancel(t.Context())) })
	n, err := h.pull(t, 1, func(context.Context, bus.DeadLetter) error { return nil })
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable || n != 0 {
		t.Fatalf("pull without DEADLETTER = %d, %v, want upstream_unavailable", n, err)
	}
}

func TestEventAt_returnsTheStoredEventAndNotFoundWhenItIsGone(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	id := h.publishPing(t)

	msg, err := h.bus.Conn.EventAt(t.Context(), 1)
	if err != nil || msg.Subject != h.bus.Conn.Subject(events.TypeSystemPinged.Subject()) ||
		msg.Header.Get(nats.MsgIdHdr) != id.String() || len(msg.Data) == 0 {
		t.Fatalf("EventAt(1) = %+v, %v, want the published ping %s", msg, err, id)
	}
	if _, err := h.bus.Conn.EventAt(t.Context(), 99); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("EventAt(99) = %v, want not_found", err)
	}
	if err := h.bus.JS.DeleteStream(t.Context(), h.bus.Events); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _, _ = h.bus.Conn.Apply(context.WithoutCancel(t.Context())) })
	if _, err := h.bus.Conn.EventAt(t.Context(), 1); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("EventAt without EVENTS = %v, want not_found", err)
	}
}

func TestRedeliver_republishesTheEventAnAdvisoryPointsAtUnderItsOriginalEventID(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	id := h.publishPing(t)
	advisory := bus.DeadLetter{Seq: 7, Consumer: "notify", Advisory: json.RawMessage(`{"stream_seq":1}`)}

	if err := h.bus.Conn.Redeliver(t.Context(), advisory, "retry"); err != nil {
		t.Fatal(err)
	}
	again, err := h.bus.Conn.EventAt(t.Context(), 2)
	if err != nil || again.Header.Get(bus.EventIDHeader) != id.String() ||
		again.Subject != h.bus.Conn.Subject(events.TypeSystemPinged.Subject()) {
		t.Fatalf("EventAt(2) = %+v, %v, want the ping %s republished", again, err, id)
	}
	gone := bus.DeadLetter{Seq: 8, Consumer: "notify", Advisory: json.RawMessage(`{"stream_seq":99}`)}
	if err := h.bus.Conn.Redeliver(t.Context(), gone, "retry"); errs.CodeOf(err) != errs.CodeNotFound {
		t.Fatalf("Redeliver of a gone event = %v, want not_found", err)
	}
}
