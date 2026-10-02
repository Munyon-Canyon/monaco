package bus

import "sync"

type inflight struct {
	mu      sync.Mutex
	drained sync.Cond
	running int
	closed  bool
}

func newInflight() *inflight {
	in := &inflight{}
	in.drained.L = &in.mu
	return in
}

func (in *inflight) enter() bool {
	in.mu.Lock()
	defer in.mu.Unlock()
	if in.closed {
		return false
	}
	in.running++
	return true
}

func (in *inflight) leave() {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.running--
	if in.running == 0 {
		in.drained.Broadcast()
	}
}

func (in *inflight) close() {
	in.mu.Lock()
	defer in.mu.Unlock()
	in.closed = true
	for in.running > 0 {
		in.drained.Wait()
	}
}
