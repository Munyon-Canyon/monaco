package testkit

import (
	"bytes"
	"sync"
)

type Logs struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *Logs) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	n, _ := l.buf.Write(p)
	return n, nil
}

func (l *Logs) Bytes() []byte {
	l.mu.Lock()
	defer l.mu.Unlock()
	return bytes.Clone(l.buf.Bytes())
}
