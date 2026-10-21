package bus_test

import (
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestConsumerBacklog_countsUndeliveredAndUnackedMessagesAcrossConsumers(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	gen := testkit.NewIDs(1)
	now := time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC)

	if got, err := b.Conn.ConsumerBacklog(t.Context(), now); err != nil || got != (bus.ConsumerBacklog{}) {
		t.Fatalf("ConsumerBacklog with no consumers = %+v, %v; want zero", got, err)
	}

	cfg := b.Consumer
	cfg.Durable = "backlog_test"
	c, err := b.JS.CreateOrUpdateConsumer(t.Context(), b.Events, cfg)
	if err != nil {
		t.Fatal(err)
	}
	for range 3 {
		if err := b.Conn.Publish(t.Context(), pinged, []byte(`{"v":1}`), eventID(t, gen)); err != nil {
			t.Fatal(err)
		}
	}
	if got, err := b.Conn.ConsumerBacklog(t.Context(), now); err != nil || got.Pending != 3 {
		t.Fatalf("ConsumerBacklog with 3 undelivered = %+v, %v; want 3 pending", got, err)
	}

	held := fetchOne(t, c)
	if got, err := b.Conn.ConsumerBacklog(t.Context(), now); err != nil || got.Pending != 3 {
		t.Fatalf("ConsumerBacklog with 2 undelivered and 1 unacked = %+v, %v; want 3 pending", got, err)
	}

	if err := held(); err != nil {
		t.Fatal(err)
	}
	testkit.Eventually(t, func() bool {
		got, err := b.Conn.ConsumerBacklog(t.Context(), now)
		return err == nil && got.Pending == 2
	}, 5*time.Second)
}

func fetchOne(t *testing.T, c jetstream.Consumer) func() error {
	t.Helper()
	batch, err := c.Fetch(1, jetstream.FetchMaxWait(5*time.Second))
	if err != nil {
		t.Fatal(err)
	}
	var held []jetstream.Msg
	for m := range batch.Messages() {
		held = append(held, m)
	}
	if len(held) != 1 {
		t.Fatalf("fetched %d messages, want 1", len(held))
	}
	return held[0].Ack
}

func TestConsumerBacklog_failsAsUnavailableWhenTheEventsStreamIsMissing(t *testing.T) {
	t.Parallel()
	b := testkit.NATS(t)
	if err := b.JS.DeleteStream(t.Context(), b.Events); err != nil {
		t.Fatal(err)
	}
	_, err := b.Conn.ConsumerBacklog(t.Context(), time.Date(2026, 3, 1, 12, 0, 0, 0, time.UTC))
	if errs.CodeOf(err) != errs.CodeUpstreamUnavailable {
		t.Fatalf("ConsumerBacklog without the stream = %v, want upstream_unavailable", err)
	}
	if _, err := b.Conn.Apply(t.Context()); err != nil {
		t.Fatal(err)
	}
}
