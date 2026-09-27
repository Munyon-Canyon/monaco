package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestSubscribeHints_deliversTheKeyAfterTheNamespacedHintPrefix(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	got := make(chan string, 1)
	if err := b.Conn.SubscribeHints(t.Context(), func(_ context.Context, key string) { got <- key }); err != nil {
		t.Fatal(err)
	}
	if n := testkit.NATSSubscriptions(t, b.Conn.Subject("hint.>")); n != 1 {
		t.Fatalf("subscriptions on %s = %d, want 1", b.Conn.Subject("hint.>"), n)
	}

	b.Conn.PublishHint(t.Context(), "cabal.42.updated", nil)
	select {
	case key := <-got:
		if key != "cabal.42.updated" {
			t.Fatalf("key = %q, want cabal.42.updated", key)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("hint not delivered within 5s")
	}
}

func TestSubscribeHints_failsOnAClosedConnection(t *testing.T) {
	t.Parallel()
	conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessAPI)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close(t.Context())
	err = conn.SubscribeHints(t.Context(), func(context.Context, string) {})
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("SubscribeHints on a closed connection = %v, want upstream_unavailable", err)
	}
}
