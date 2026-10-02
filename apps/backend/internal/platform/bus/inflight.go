package bus

import "sync"

type inflight struct {
	mu     sync.Mutex
	n      int
	closed bool
	done   chan struct{}
}

func newInflight() *inflight { return &inflight{} }

func (in *inflight) enter() bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.closed {
		return false
	}
	if in.n == 0 {
		in.done = make(chan struct{})
	}
	in.n++
	return true
}

func (in *inflight) leave() {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.n--
	if in.n == 0 {
		close(in.done)
	}
}

func (in *inflight) close() {
	in.mu.Lock()
	in.closed = true
	if in.n == 0 {
		in.mu.Unlock()
		return
	}
	done := in.done
	in.mu.Unlock()
	<-done
}

func (in *inflight) run(fn func()) {
	if !in.enter() {
		return
	}
	defer in.leave()
	fn()
}
