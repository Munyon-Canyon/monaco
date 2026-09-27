package testkit

import "sync"

type Faults struct {
	mu     sync.Mutex
	always map[string]error
	once   map[string][]error
}

func (f *Faults) Fail(op string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.always == nil {
		f.always = map[string]error{}
	}
	f.always[op] = err
}

func (f *Faults) FailOnce(op string, err error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.once == nil {
		f.once = map[string][]error{}
	}
	f.once[op] = append(f.once[op], err)
}

func (f *Faults) Check(op string) error {
	f.mu.Lock()
	defer f.mu.Unlock()
	if queued := f.once[op]; len(queued) > 0 {
		f.once[op] = queued[1:]
		return queued[0]
	}
	return f.always[op]
}
