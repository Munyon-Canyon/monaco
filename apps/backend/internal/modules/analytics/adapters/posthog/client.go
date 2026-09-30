package posthog

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"log/slog"
	"maps"
	"net/http"
	"time"

	"github.com/google/uuid"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpclient"
)

const (
	upstream   = "posthog"
	batchPath  = "/batch/"
	setKey     = "$set"
	maxDrained = 64 << 10
)

type Client struct {
	http   *httpclient.Client
	apiKey string
}

func New(cfg config.Config, newHTTP func(name string, opts ...httpclient.Option) *httpclient.Client) Client {
	return Client{
		http: newHTTP(upstream,
			httpclient.WithBaseURL(cfg.PostHog.Host),
			httpclient.WithTimeout(cfg.Timeouts.PostHog),
		),
		apiKey: cfg.PostHog.APIKey,
	}
}

type event struct {
	UUID       uuid.UUID      `json:"uuid"`
	Event      string         `json:"event"`
	DistinctID string         `json:"distinct_id"`
	Timestamp  time.Time      `json:"timestamp"`
	Properties map[string]any `json:"properties"`
}

func wire(c app.Capture) event {
	props := maps.Clone(c.Properties)
	if props == nil {
		props = map[string]any{}
	}
	if len(c.Set) > 0 {
		props[setKey] = c.Set
	}
	return event{
		UUID: c.UUID, Event: c.Event, DistinctID: c.DistinctID, Timestamp: c.Timestamp.UTC(), Properties: props,
	}
}

func (c Client) Capture(ctx context.Context, batch []app.Capture) error {
	const op = "posthog.Client.Capture"
	events := make([]event, len(batch))
	for i, capture := range batch {
		events[i] = wire(capture)
	}
	raw, err := json.Marshal(map[string]any{"api_key": c.apiKey, "batch": events})
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, op)
	}
	req, _ := http.NewRequestWithContext(ctx, http.MethodPost, batchPath, bytes.NewReader(raw))
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.http.Do(ctx, req)
	if err != nil {
		return errs.Wrap(err, errs.CodePostHogUnavailable, op)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, io.LimitReader(resp.Body, maxDrained))
	return statusError(resp.StatusCode, op)
}

func statusError(status int, op string) error {
	switch {
	case status >= http.StatusOK && status < http.StatusMultipleChoices:
		return nil
	case status >= http.StatusInternalServerError:
		return errs.New(errs.CodePostHogUnavailable, op, slog.Int("status", status))
	default:
		return errs.New(errs.CodePostHogRejected, op, slog.Int("status", status))
	}
}
