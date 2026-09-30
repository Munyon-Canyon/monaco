package posthog_test

import (
	"bytes"
	"encoding/json"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/adapters/posthog"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func logged(t *testing.T, raw []byte) []map[string]any {
	t.Helper()
	var out []map[string]any
	dec := json.NewDecoder(bytes.NewReader(raw))
	for dec.More() {
		var line map[string]any
		if err := dec.Decode(&line); err != nil {
			t.Fatal(err)
		}
		out = append(out, line)
	}
	return out
}

func TestNoop_Capture_logsEachSkippedCaptureAndSucceeds(t *testing.T) {
	t.Parallel()
	var logs testkit.Logs
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvLocal}, &logs))
	first, second := sample(), sample()
	second.UUID, second.Event = testkit.NewIDs(2).NewV7(), "other"
	if err := (posthog.Noop{}).Capture(ctx, []app.Capture{first, second}); err != nil {
		t.Fatal(err)
	}
	lines := logged(t, logs.Bytes())
	if len(lines) != 2 {
		t.Fatalf("logged %d lines, want 2: %s", len(lines), logs.Bytes())
	}
	for i, c := range []app.Capture{first, second} {
		line := lines[i]
		if line["msg"] != "analytics.capture_skipped" || line["reason"] != "no_api_key" ||
			line["event"] != c.Event || line["uuid"] != c.UUID.String() {
			t.Errorf("line %d = %v, want analytics.capture_skipped for %s %s", i, line, c.Event, c.UUID)
		}
	}
}

func TestNoop_Capture_withAnEmptyBatchLogsNothing(t *testing.T) {
	t.Parallel()
	var logs testkit.Logs
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{Env: config.EnvLocal}, &logs))
	if err := (posthog.Noop{}).Capture(ctx, nil); err != nil || len(logs.Bytes()) != 0 {
		t.Fatalf("Capture(nil) = %v with logs %q, want success and none", err, logs.Bytes())
	}
}
