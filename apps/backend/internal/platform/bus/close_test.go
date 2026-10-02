package bus_test

import (
	"context"
	"testing"
	"time"

	"github.com/nats-io/nats.go"
	"go.uber.org/goleak"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestClose_liveContextLeavesNoGoroutine(t *testing.T) {
	before := goleak.IgnoreCurrent()
	conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessMonacoctl)
	if err != nil {
		t.Fatal(err)
	}
	nc := conn.NATS()
	entered, release := make(chan struct{}), make(chan struct{})
	if _, err := nc.Subscribe("close.block", func(*nats.Msg) {
		close(entered)
		<-release
	}); err != nil {
		t.Fatal(err)
	}
	if err := nc.Publish("close.block", nil); err != nil {
		t.Fatal(err)
	}
	await(t, "the subscriber to block", entered)
	conn.Close(t.Context())
	if !nc.IsClosed() {
		t.Fatal("Close returned with the connection still open")
	}
	close(release)
	goleak.VerifyNone(t, before)
}

func TestClose_expiredContextLeavesNoGoroutine(t *testing.T) {
	before := goleak.IgnoreCurrent()
	const flushCap = 100 * time.Millisecond
	conn, err := bus.Connect(
		t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessMonacoctl, bus.WithCloseFlushTimeout(flushCap),
	)
	if err != nil {
		t.Fatal(err)
	}
	nc := conn.NATS()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	started := time.Now()
	conn.Close(ctx)
	if elapsed := time.Since(started); elapsed >= flushCap {
		t.Fatalf("Close took %s after the context expired, want it back inside the flush cap", elapsed)
	}
	if !nc.IsClosed() {
		t.Fatal("Close returned with the connection still open")
	}
	goleak.VerifyNone(t, before)
}

func TestConnect_reportsTheLiveConnectionAndAnUnprefixedStream(t *testing.T) {
	conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessMonacoctl)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { conn.Close(t.Context()) })
	if !conn.Connected() {
		t.Fatal("connected connection reports not connected")
	}
	if got := conn.Stream("ORDERS"); got != "ORDERS" {
		t.Fatalf("Stream() = %q, want ORDERS", got)
	}
}
