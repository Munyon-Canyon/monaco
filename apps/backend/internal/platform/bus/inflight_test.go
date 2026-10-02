package bus

import (
	"sync"
	"testing"
	"testing/synctest"
)

func TestInflightCloseBlocksUntilLeave(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		in := newInflight()
		if !in.enter() {
			t.Fatal("enter")
		}
		returned := make(chan struct{})
		go func() {
			in.close()
			close(returned)
		}()
		synctest.Wait()
		select {
		case <-returned:
			t.Fatal("close returned while a dispatch was in flight")
		default:
		}
		in.leave()
		<-returned
	})
}

func TestInflightEnterAfterCloseReturnsFalse(t *testing.T) {
	t.Parallel()
	in := newInflight()
	in.close()
	if in.enter() {
		t.Fatal("enter after close")
	}
	ran := false
	in.run(func() { ran = true })
	if ran {
		t.Fatal("dispatch ran after close")
	}
}

func TestInflightConcurrentEnterLeaveLetsCloseReturn(t *testing.T) {
	t.Parallel()
	in := newInflight()
	if !in.enter() {
		t.Fatal("enter")
	}
	if !in.enter() {
		t.Fatal("second enter")
	}
	in.leave()
	var wg sync.WaitGroup
	wg.Add(100)
	for range 100 {
		go func() {
			defer wg.Done()
			if in.enter() {
				in.leave()
			}
		}()
	}
	in.leave()
	in.close()
	wg.Wait()
	if in.enter() {
		t.Fatal("enter after close")
	}
}
