package observability

import (
	"bytes"
	"encoding/json"
	"errors"
	"io"
	"log/slog"
	"testing"
	"time"

	"go.opentelemetry.io/otel/trace"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func decodeLines(t *testing.T, buf *bytes.Buffer) []map[string]any {
	t.Helper()
	var lines []map[string]any
	dec := json.NewDecoder(buf)
	for dec.More() {
		var line map[string]any
		if err := dec.Decode(&line); err != nil {
			t.Fatal(err)
		}
		lines = append(lines, line)
	}
	return lines
}

func TestNewLogger_contextCarriesJoinKeysWithoutTheCaller(t *testing.T) {
	t.Parallel()
	eventID, err := ids.ParseEventID(ids.Real{}.NewV7().String())
	if err != nil {
		t.Fatal(err)
	}
	traceID, err := trace.TraceIDFromHex("4bf92f3577b34da6a3ce929d0e0e4736")
	if err != nil {
		t.Fatal(err)
	}
	spanID, err := trace.SpanIDFromHex("00f067aa0ba902b7")
	if err != nil {
		t.Fatal(err)
	}
	sc := trace.NewSpanContext(trace.SpanContextConfig{TraceID: traceID, SpanID: spanID})
	ctx := trace.ContextWithSpanContext(t.Context(), sc)
	ctx = WithRequestID(ctx, "req-1")
	ctx = WithActor(ctx, "user:u1")
	ctx = WithModule(ctx, "treasury")
	ctx = WithOp(ctx, "treasury.FundCabal")
	ctx = WithEventID(ctx, eventID)

	var buf bytes.Buffer
	logger := NewLogger(config.Config{Env: config.EnvProduction}, &buf)
	logger.InfoContext(ctx, "treasury.fund.rejected", slog.Int64("have", 4_000_000))
	logger.WithGroup("fund").InfoContext(ctx, "treasury.fund.rejected", slog.Int64("need", 5_000_000))

	lines := decodeLines(t, &buf)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	want := map[string]any{
		"trace_id":   "4bf92f3577b34da6a3ce929d0e0e4736",
		"span_id":    "00f067aa0ba902b7",
		"request_id": "req-1",
		"event_id":   eventID.String(),
		"actor":      "user:u1",
		"module":     "treasury",
		"op":         "treasury.FundCabal",
	}
	for i, line := range lines {
		for k, v := range want {
			if line[k] != v {
				t.Errorf("line %d %s = %v, want %v", i, k, line[k], v)
			}
		}
	}
	if lines[0]["have"] != float64(4_000_000) {
		t.Errorf("have = %v, want 4000000", lines[0]["have"])
	}
	if fund, _ := lines[1]["fund"].(map[string]any); fund["need"] != float64(5_000_000) {
		t.Errorf("fund = %v, want the record attrs nested under the group", lines[1]["fund"])
	}
}

func TestNewLogger_bareContextAddsNoJoinKeys(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	NewLogger(config.Config{}, &buf).InfoContext(t.Context(), "poller.tick")
	lines := decodeLines(t, &buf)
	for _, k := range []string{"trace_id", "span_id", "request_id", "event_id", "actor", "module", "op"} {
		if _, ok := lines[0][k]; ok {
			t.Errorf("line has %s, want it absent", k)
		}
	}
}

func TestNewLogger_debugOnlyOutsideProduction(t *testing.T) {
	t.Parallel()
	for env, wantLines := range map[config.Env]int{
		config.EnvLocal:      1,
		config.EnvTest:       1,
		config.EnvStaging:    0,
		config.EnvProduction: 0,
	} {
		var buf bytes.Buffer
		NewLogger(config.Config{Env: env}, &buf).DebugContext(t.Context(), "step")
		if got := len(decodeLines(t, &buf)); got != wantLines {
			t.Errorf("env %s: %d debug lines, want %d", env, got, wantLines)
		}
	}
}

func TestNewLogger_groupsNestAttrsInOrder(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := NewLogger(config.Config{}, &buf).
		With(slog.String("service", "api")).
		WithGroup("a").
		With(slog.String("x", "1")).
		WithGroup("b").
		With(slog.String("y", "2"))
	logger.InfoContext(WithRequestID(t.Context(), "r"), "m", slog.String("z", "3"))
	logger.WithGroup("empty").InfoContext(t.Context(), "m")

	lines := decodeLines(t, &buf)
	got, err := json.Marshal([]any{lines[0]["service"], lines[0]["request_id"], lines[0]["a"], lines[1]["a"]})
	if err != nil {
		t.Fatal(err)
	}
	want := `["api","r",{"b":{"y":"2","z":"3"},"x":"1"},{"b":{"y":"2"},"x":"1"}]`
	if string(got) != want {
		t.Fatalf("got %s, want %s", got, want)
	}
}

func TestHandler_emptyGroupReturnsReceiver(t *testing.T) {
	t.Parallel()
	h := NewLogger(config.Config{}, &bytes.Buffer{}).Handler()
	if h.WithGroup("") != h {
		t.Fatal("WithGroup(\"\") returned a new handler, want the receiver")
	}
}

type failingWriter struct{}

func (failingWriter) Write([]byte) (int, error) { return 0, io.ErrClosedPipe }

func TestHandler_writeFailureIsInternal(t *testing.T) {
	t.Parallel()
	h := NewLogger(config.Config{}, failingWriter{}).Handler()
	err := h.Handle(t.Context(), slog.NewRecord(time.Time{}, slog.LevelInfo, "m", 0))
	if !errors.Is(err, io.ErrClosedPipe) || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("Handle = %v, want an internal error wrapping io.ErrClosedPipe", err)
	}
}
