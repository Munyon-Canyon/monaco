package bus_test

import (
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
