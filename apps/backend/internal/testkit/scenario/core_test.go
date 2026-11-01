package scenario

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
)

func TestCoreStepsReadTheSubscribedMessage(t *testing.T) {
	t.Parallel()
	messages := make(chan []byte, 1)
	messages <- []byte(`{"prices":3}`)
	s := Against(t.Context(), t, Remote{
		Enter:         func(Stage) {},
		Logs:          (&lineLog{note: newNotifier()}).since,
		CoreSubscribe: func(T, string) <-chan []byte { return messages },
	})
	s.When(SubscribeCore("price.tick"), ExpectCore("price.tick", func(data []byte) error {
		if string(data) != `{"prices":3}` {
			return errors.New("wrong price tick")
		}
		return nil
	}))
}

func TestExpectCoreRequiresASubscription(t *testing.T) {
	t.Parallel()
	got := failure(t, t.Context, func(r T) *Scenario {
		return Against(t.Context(), r, Remote{
			Enter:         func(Stage) {},
			Logs:          (&lineLog{note: newNotifier()}).since,
			CoreSubscribe: func(T, string) <-chan []byte { return make(chan []byte) },
		})
	}, ExpectCore("price.tick", func([]byte) error { return nil }))
	const want = "scenario: ExpectCore(price.tick) needs SubscribeCore(price.tick) first"
	if got != want {
		t.Fatalf("failure = %q, want %q", got, want)
	}
}

func publishedScenario(r T, published func() uint64) *Scenario {
	return newScenario(r, &backend{
		enter:     func(Stage) {},
		events:    func(T, events.Type, []string) []string { return nil },
		published: func(T, events.Type, []string) uint64 { return published() },
	})
}

func TestExpectPublished_waitsForTheRelayToMarkTheRowPublished(t *testing.T) {
	t.Parallel()
	var reads atomic.Uint64
	got := failure(t, t.Context, func(r T) *Scenario {
		return publishedScenario(r, func() uint64 { return reads.Add(1) - 1 })
	}, ExpectPublished(events.TypeSystemPinged, 2))
	if got != "" {
		t.Fatalf("failure = %q, want none once the count reached 2", got)
	}
	if reads.Load() < 3 {
		t.Fatalf("published read %d times, want it polled until the count reached 2", reads.Load())
	}
}

func TestExpectPublished_failsWhenTheCountPassesWhatWasExpected(t *testing.T) {
	t.Parallel()
	const want = "scenario: 1 system.pinged messages on the stream, want 0"
	got := failure(t, t.Context, func(r T) *Scenario {
		return publishedScenario(r, func() uint64 { return 1 })
	}, ExpectPublished(events.TypeSystemPinged, 0))
	if got != want {
		t.Fatalf("failure = %q, want %q", got, want)
	}
}

func TestExpectPublished_failsWhenTheCountNeverReachesWhatWasExpected(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithTimeout(t.Context(), 100*time.Millisecond)
	defer cancel()
	const want = "scenario: 0 system.pinged messages on the stream, want 1"
	got := failure(t, func() context.Context { return ctx }, func(r T) *Scenario {
		return publishedScenario(r, func() uint64 { return 0 })
	}, ExpectPublished(events.TypeSystemPinged, 1))
	if got != want {
		t.Fatalf("failure = %q, want %q", got, want)
	}
}
