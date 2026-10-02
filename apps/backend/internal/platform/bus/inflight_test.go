package bus

import (
	"sync"
	"testing"
	"testing/synctest"
)

func TestInflight_closeWaitsForTheRunningDispatchToLeave(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		in := newInflight()
		if !in.enter() {
			t.Fatal("enter before close = false, want true")
		}
		closed := make(chan struct{})
		go func() {
			defer close(closed)
			in.close()
		}()
		synctest.Wait()
		select {
		case <-closed:
			t.Fatal("close returned while a dispatch was still running")
		default:
		}
		in.leave()
		<-closed
	})
}

func TestInflight_enterAfterCloseIsRefused(t *testing.T) {
	t.Parallel()
	in := newInflight()
	in.close()
	if in.enter() {
		t.Fatal("enter after close = true, want false")
	}
}

func TestInflight_closeReturnsAfterConcurrentDispatchesLeave(t *testing.T) {
	t.Parallel()
	in := newInflight()
	var wg sync.WaitGroup
	for range 100 {
		wg.Go(func() {
			if in.enter() {
				in.leave()
			}
		})
	}
	wg.Wait()
	in.close()
	if in.running != 0 {
		t.Fatalf("running = %d after every dispatch left, want 0", in.running)
	}
}
