package bus_test

import (
	"context"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestRegistryStop_waitsForADispatchStillRunning(t *testing.T) {
	t.Parallel()
	h := newHarness(t)
	started := make(chan struct{})
	release := make(chan struct{})
	var once sync.Once
	letGo := func() { once.Do(func() { close(release) }) }
	var startedOnce sync.Once
	var finished atomic.Bool
	slow := bus.Handle("notify.push", func(context.Context, db.Tx, events.SystemPinged, time.Time) error {
		startedOnce.Do(func() { close(started) })
		<-release
		finished.Store(true)
		return nil
	})
	reg := h.registry(t, bus.Consumer{Durable: durable, Handlers: []bus.HandlerSpec{slow}})
	stop, err := reg.Start(h.ctx(t))
	if err != nil {
		t.Fatal(err)
	}
	h.publishPing(t)
	testkit.Eventually(t, func() bool {
		select {
		case <-started:
			return true
		default:
			return false
		}
	}, waitLong)
	reg.SetBeforeClosed(func() {
		for range 1000 {
			runtime.Gosched()
		}
	})
	done := make(chan struct{})
	var stoppedEarly atomic.Bool
	go func() {
		stop()
		stoppedEarly.Store(!finished.Load())
		close(done)
	}()
	defer func() {
		letGo()
		<-done
	}()
	for range 20000 {
		select {
		case <-done:
			if stoppedEarly.Load() {
				t.Fatal("stop returned before the handler")
			}
			return
		default:
			runtime.Gosched()
		}
	}
	letGo()
	<-done
	if stoppedEarly.Load() {
		t.Fatal("stop returned before the handler")
	}
}
