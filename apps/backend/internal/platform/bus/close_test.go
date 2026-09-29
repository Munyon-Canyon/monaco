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

func TestClose_stopsWaitingForTheDrainWhenTheContextEnds(t *testing.T) {
	before := goleak.IgnoreCurrent()
	conn, err := bus.Connect(t.Context(), config.NATS{URL: testkit.NATSURL()}, bus.ProcessMonacoctl)
	if err != nil {
		t.Fatal(err)
	}
	nc := conn.NATS()
	nc.Opts.DrainTimeout = 100 * time.Millisecond
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
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	conn.Close(ctx)
	if !nc.IsClosed() {
		t.Fatal("Close returned with the connection still open after its context ended")
	}
	close(release)
	testkit.Eventually(t, func() bool {
		endTheDrainThatReopensAfterClose(nc)
		return goleak.Find(before) == nil
	}, 10*time.Second)
}

func endTheDrainThatReopensAfterClose(nc *nats.Conn) { nc.Close() }
