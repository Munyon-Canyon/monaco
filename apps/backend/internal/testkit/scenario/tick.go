package scenario

import (
	"bytes"
	"encoding/json"
	"slices"
	"sync"

	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type tick struct {
	code             string
	scanned, changed int
}

func AwaitTick(poller string) Step {
	return func(s *Scenario) {
		s.t.Helper()
		seen, _ := s.app.lines(0)
		from := len(seen)
		stop := s.app.tick(s.t, poller)
		defer stop()
		var found tick
		await(s.t, "a tick of poller "+poller+" after the step started", func() (bool, <-chan struct{}) {
			lines, changed := s.app.lines(from)
			from += len(lines)
			var ok bool
			found, ok = firstTick(lines, poller)
			return ok, changed
		})
		s.ticks[poller] = found
	}
}

func ExpectTick(poller string, scanned, changed int) Step {
	return func(s *Scenario) {
		s.t.Helper()
		got, ok := s.ticks[poller]
		switch {
		case !ok:
			s.t.Fatalf("scenario: ExpectTick(%s) needs AwaitTick(%s) first", poller, poller)
		case got.code != "":
			s.t.Fatalf("scenario: poller %s failed with code %s, want scanned %d changed %d",
				poller, got.code, scanned, changed)
		case got.scanned != scanned || got.changed != changed:
			s.t.Fatalf("scenario: poller %s scanned %d changed %d, want scanned %d changed %d",
				poller, got.scanned, got.changed, scanned, changed)
		}
	}
}

func firstTick(lines []string, poller string) (tick, bool) {
	for _, line := range lines {
		var f struct {
			Msg     string `json:"msg"`
			Poller  string `json:"poller"`
			Code    string `json:"code"`
			Scanned int    `json:"scanned"`
			Changed int    `json:"changed"`
		}
		if json.Unmarshal([]byte(line), &f) != nil || f.Poller != poller {
			continue
		}
		switch f.Msg {
		case observability.PollerTick.Name:
			return tick{scanned: f.Scanned, changed: f.Changed}, true
		case observability.PollerFailed.Name:
			return tick{code: f.Code}, true
		}
	}
	return tick{}, false
}

type lineLog struct {
	note    *notifier
	mu      sync.Mutex
	pending []byte
	lines   []string
}

func (l *lineLog) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	l.pending = append(l.pending, p...)
	for {
		i := bytes.IndexByte(l.pending, '\n')
		if i < 0 {
			return len(p), nil
		}
		line := string(l.pending[:i])
		l.pending = l.pending[i+1:]
		l.note.update(func() { l.lines = append(l.lines, line) })
	}
}

func (l *lineLog) since(from int) ([]string, <-chan struct{}) {
	l.note.mu.Lock()
	defer l.note.mu.Unlock()
	return slices.Clone(l.lines[from:]), l.note.changed
}
