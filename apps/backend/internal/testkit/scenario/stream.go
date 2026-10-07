package scenario

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"sync"
	"time"

	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
)

const (
	convergeWithin = 30 * time.Second
	tokenKey       = "scenario"
)

type notifier struct {
	mu      sync.Mutex
	changed chan struct{}
}

func newNotifier() *notifier { return &notifier{changed: make(chan struct{})} }

func (n *notifier) update(change func()) {
	n.mu.Lock()
	defer n.mu.Unlock()
	change()
	close(n.changed)
	n.changed = make(chan struct{})
}

func (n *notifier) await(t T, what string, done func() bool) {
	t.Helper()
	await(t, what, func() (bool, <-chan struct{}) {
		n.mu.Lock()
		defer n.mu.Unlock()
		return done(), n.changed
	})
}

func await(t T, what string, poll func() (done bool, changed <-chan struct{})) {
	t.Helper()
	deadline := time.NewTimer(convergeWithin)
	defer deadline.Stop()
	for {
		ok, changed := poll()
		if ok {
			return
		}
		select {
		case <-changed:
		case <-deadline.C:
			t.Fatalf("scenario: %s did not happen within %s", what, convergeWithin)
		case <-t.Context().Done():
			t.Fatalf("scenario: %s did not happen: %v", what, context.Cause(t.Context()))
		}
	}
}

func background(ctx context.Context, run func(context.Context)) func() {
	ctx, cancel := context.WithCancel(ctx)
	var g errgroup.Group
	g.Go(func() error {
		run(ctx)
		return nil
	})
	return func() {
		cancel()
		_ = g.Wait()
	}
}

type stream struct {
	hints []sse.Hint
}

func openStream(t T, b *backend, token string) *stream {
	t.Helper()
	s := &stream{}
	opened := make(chan error, 1)
	t.Cleanup(background(context.WithoutCancel(t.Context()), func(ctx context.Context) {
		follow(ctx, b, s, token, opened)
	}))
	if err := <-opened; err != nil {
		t.Fatalf("scenario: open /v1/stream: %v", err)
	}
	return s
}

type statusError int

func (s statusError) Error() string { return "answered " + http.StatusText(int(s)) }

func follow(ctx context.Context, b *backend, s *stream, token string, opened chan<- error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, b.baseURL+"/v1/stream", nil)
	if err != nil {
		opened <- err
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := b.client.Do(req)
	if err != nil {
		opened <- err
		return
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		opened <- statusError(resp.StatusCode)
		return
	}
	opened <- nil
	lines := bufio.NewScanner(resp.Body)
	event := ""
	for lines.Scan() {
		line := lines.Text()
		if name, ok := strings.CutPrefix(line, "event: "); ok {
			event = name
			continue
		}
		data, ok := strings.CutPrefix(line, "data: ")
		if !ok || event != "hint" {
			continue
		}
		var h sse.Hint
		if json.Unmarshal([]byte(data), &h) == nil {
			b.note.update(func() { s.hints = append(s.hints, h) })
		}
	}
}
