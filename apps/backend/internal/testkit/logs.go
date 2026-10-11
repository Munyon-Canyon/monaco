package testkit

import (
	"bytes"
	"encoding/json"
	"fmt"
	"strings"
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

func (l *Logs) Lines(msg string, keys ...string) []string {
	var out []string
	for line := range strings.Lines(string(l.Bytes())) {
		var entry map[string]any
		if json.Unmarshal([]byte(line), &entry) != nil || entry["msg"] != msg {
			continue
		}
		values := make([]string, len(keys))
		for i, k := range keys {
			values[i] = fmt.Sprint(entry[k])
		}
		out = append(out, strings.Join(values, " "))
	}
	return out
}
