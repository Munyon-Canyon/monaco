package scenario

import (
	"bufio"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/sse"
)

type stream struct {
	hints []sse.Hint
}

func (a *app) openStream(t *testing.T, token string) *stream {
	t.Helper()
	s := &stream{}
	opened := make(chan error, 1)
	t.Cleanup(background(context.WithoutCancel(t.Context()), func(ctx context.Context) {
		a.follow(ctx, s, token, opened)
	}))
	if err := <-opened; err != nil {
		t.Fatalf("scenario: open /v1/stream: %v", err)
	}
	return s
}

type statusError int

func (s statusError) Error() string { return "answered " + http.StatusText(int(s)) }

func (a *app) follow(ctx context.Context, s *stream, token string, opened chan<- error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, a.server.URL+"/v1/stream", nil)
	if err != nil {
		opened <- err
		return
	}
	req.Header.Set("Authorization", "Bearer "+token)
	resp, err := a.server.Client().Do(req)
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
			a.update(func() { s.hints = append(s.hints, h) })
		}
	}
}
