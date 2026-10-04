package scenario

import (
	"errors"
	"testing"
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
