package bus_test

import (
	"context"
	"encoding/json"
	"testing"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestPublishCore_sendsTheMessageOnItsNamespacedCoreSubject(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	sub, err := b.Conn.NATS().SubscribeSync(b.Conn.Subject("price.tick"))
	if err != nil {
		t.Fatal(err)
	}
	asOf := clock.Real{}.Now().UTC().Truncate(2 * time.Minute)
	tick := events.PriceTick{V: 1, AsOf: asOf, Prices: []events.TickPrice{{
		Mint: "XsbEhLAtcf6HdfpFZ5xEMdqW8nfAvcsP5bdudRLJzJp", AssetID: uuid.Max,
		PriceMicros: money.MicrosFromUint64(254_371_234), ObservedAt: asOf,
	}}}
	if err := b.Conn.PublishCore(t.Context(), tick); err != nil {
		t.Fatal(err)
	}
	msg, err := sub.NextMsg(5 * time.Second)
	if err != nil {
		t.Fatalf("no core message on %s: %v", b.Conn.Subject("price.tick"), err)
	}
	var got events.PriceTick
	if err := json.Unmarshal(msg.Data, &got); err != nil || len(got.Prices) != 1 || !got.AsOf.Equal(asOf) ||
		got.Prices[0].PriceMicros != tick.Prices[0].PriceMicros || got.Prices[0].AssetID != uuid.Max {
		t.Fatalf("core message %s = %+v, %v, want %+v", msg.Data, got, err, tick)
	}
	if pending, _, err := sub.Pending(); err != nil || pending != 0 {
		t.Fatalf("pending after one publish = %d, %v, want exactly one message", pending, err)
	}
}

func TestPublishCore_failsOnAClosedConnection(t *testing.T) {
	t.Parallel()
	conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessWorker)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close(t.Context())
	err = conn.PublishCore(t.Context(), events.PriceTick{V: 1})
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("PublishCore on a closed connection = %v, want upstream_unavailable", err)
	}
}

func TestSubscribeCore_deliversTheNamespacedPayloadUntilUnsubscribe(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	got := make(chan []byte, 1)
	unsub, err := b.Conn.SubscribeCore(t.Context(), "price.tick", func(_ context.Context, data []byte) {
		got <- data
	})
	if err != nil {
		t.Fatal(err)
	}
	subject := b.Conn.Subject("price.tick")
	if n := testkit.NATSSubscriptions(t, subject); n != 1 {
		t.Fatalf("subscriptions on %s = %d, want 1", subject, n)
	}
	if err := b.Conn.PublishCore(t.Context(), events.PriceTick{V: 1}); err != nil {
		t.Fatal(err)
	}
	var data []byte
	testkit.Eventually(t, func() bool {
		select {
		case data = <-got:
			return true
		default:
			return false
		}
	}, 5*time.Second)
	var tick events.PriceTick
	if err := json.Unmarshal(data, &tick); err != nil || tick.V != 1 {
		t.Fatalf("payload = %s, %v, want price.tick v 1", data, err)
	}
	unsub()
	testkit.Eventually(t, func() bool { return testkit.NATSSubscriptions(t, subject) == 0 }, 5*time.Second)
}

func TestSubscribeCore_failsOnAClosedConnection(t *testing.T) {
	t.Parallel()
	conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessWorker)
	if err != nil {
		t.Fatal(err)
	}
	conn.Close(t.Context())
	unsub, err := conn.SubscribeCore(t.Context(), "price.tick", func(context.Context, []byte) {})
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("SubscribeCore on a closed connection = %v, want upstream_unavailable", err)
	}
	if unsub != nil {
		t.Fatal("SubscribeCore returned an unsubscribe func with the error")
	}
}
