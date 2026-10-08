package agents

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"os"
	"slices"
	"strings"
	"sync"
	"time"
)

type traceEntry struct {
	n int
	d time.Duration
}

type watchTrace struct {
	mu    sync.Mutex
	stats map[string]traceEntry
}

func (t *watchTrace) add(key string, d time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	e := t.stats[key]
	t.stats[key] = traceEntry{n: e.n + 1, d: e.d + d}
}

type tracingTransport struct {
	base http.RoundTripper
	t    *watchTrace
}

func (tt tracingTransport) RoundTrip(r *http.Request) (*http.Response, error) {
	start := time.Now()
	resp, err := tt.base.RoundTrip(r)
	tt.t.add("http "+r.Method+" "+r.URL.Path, time.Since(start))
	if err != nil {
		return nil, fmt.Errorf("round trip: %w", err)
	}
	return resp, nil
}

func (env *Env) traceWatch(stderr io.Writer) func() {
	if os.Getenv("MONACO_WATCH_TRACE") != "1" {
		return func() {}
	}
	t := &watchTrace{stats: map[string]traceEntry{}}
	start := time.Now()
	run := env.Run
	env.Run = func(ctx context.Context, dir, stdin, name string, args ...string) ([]byte, error) {
		begin := time.Now()
		out, err := run(ctx, dir, stdin, name, args...)
		key := name
		if len(args) > 0 {
			key += " " + args[0]
		}
		t.add(key, time.Since(begin))
		return out, err
	}
	if c := env.GitHub.HTTP; c != nil {
		base := c.Transport
		if base == nil {
			base = http.DefaultTransport
		}
		c.Transport = tracingTransport{base: base, t: t}
	}
	return func() {
		keys := make([]string, 0, len(t.stats))
		for k := range t.stats {
			keys = append(keys, k)
		}
		slices.SortFunc(keys, func(a, b string) int { return int(t.stats[b].d - t.stats[a].d) })
		var sb strings.Builder
		for _, k := range keys {
			e := t.stats[k]
			fmt.Fprintf(&sb, "trace %8s x%-3d %s\n", e.d.Round(time.Millisecond), e.n, k)
		}
		fmt.Fprintf(&sb, "trace pass total %s\n", time.Since(start).Round(time.Millisecond))
		_, _ = io.WriteString(stderr, sb.String())
	}
}
