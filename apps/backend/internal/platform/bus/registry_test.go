package bus_test

import (
	"context"
	"sync/atomic"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func closedNow(cc jetstream.ConsumeContext) bool {
	select {
	case <-cc.Closed():
		return true
	default:
		return false
	}
}

func TestRegistryStop_waitsForADispatchStillRunning(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	var cc jetstream.ConsumeContext
	halted := make(chan struct{})
	proceed := make(chan struct{})
	h.bus.Conn.AfterConsumeStop(func(stopped jetstream.ConsumeContext) {
		cc = stopped
		close(halted)
		<-proceed
	})
	entered := make(chan struct{})
	release := make(chan struct{})
	var returned atomic.Bool
	blocking := bus.Handle("notify.push",
		func(ctx context.Context, tx db.Tx, e events.SystemPinged, _ time.Time) error {
			close(entered)
			<-release
			_, err := tx.Queries().Exec(ctx,
				`INSERT INTO handled (handler, event_id) VALUES ('notify.push', $1)`, e.PingID)
			returned.Store(true)
			return err
		})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{blocking}})
	stop, err := reg.Start(h.ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	h.publishPing(t)
	await(t, "the handler to start", entered)

	stopped := make(chan struct{})
	go func() {
		defer close(stopped)
		stop()
	}()
	await(t, "stop to stop the consume context", halted)
	testkit.Eventually(t, func() bool { return closedNow(cc) }, waitLong)
	close(proceed)
	close(release)
	await(t, "stop to return", stopped)
	if !returned.Load() {
		t.Fatal("stop returned while the handler was still running, want it to wait for the dispatch")
	}
}
