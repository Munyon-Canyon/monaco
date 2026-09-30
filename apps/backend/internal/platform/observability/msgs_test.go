package observability

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
)

func TestInfoAndDebug_logTheRegisteredNameThroughTheContextLogger(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	ctx := WithLogger(WithRequestID(t.Context(), "req-1"), NewLogger(config.Config{Env: config.EnvLocal}, &buf))
	Info(ctx, BootListening, slog.String("service", "api"), slog.String("addr", ":8080"))
	Debug(ctx, BootConfig, slog.String("service", "api"), slog.Any("config", map[string]string{"token": "t"}))

	lines := decodeLines(t, &buf)
	if len(lines) != 2 {
		t.Fatalf("got %d lines, want 2", len(lines))
	}
	for i, want := range []map[string]any{
		{"level": "INFO", "msg": "boot.listening", "request_id": "req-1", "addr": ":8080"},
		{"level": "DEBUG", "msg": "boot.config", "request_id": "req-1", "service": "api"},
	} {
		for k, v := range want {
			if lines[i][k] != v {
				t.Errorf("line %d %s = %v, want %v", i, k, lines[i][k], v)
			}
		}
	}
}

func TestDegraded_logsAtWarnThroughTheContextLogger(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	ctx := WithLogger(WithRequestID(t.Context(), "req-2"), NewLogger(config.Config{Env: config.EnvLocal}, &buf))
	Degraded(ctx, BootListening, slog.String("service", "api"), slog.String("addr", ":8080"))
	lines := decodeLines(t, &buf)
	if len(lines) != 1 || lines[0]["level"] != "WARN" || lines[0]["msg"] != "boot.listening" ||
		lines[0]["request_id"] != "req-2" || lines[0]["addr"] != ":8080" {
		t.Fatalf("lines = %v, want one WARN boot.listening line with its attrs", lines)
	}
}

func TestInfo_withoutLoggerInContextWritesNothing(t *testing.T) {
	t.Parallel()
	Info(t.Context(), BootListening, slog.String("service", "api"), slog.String("addr", ":8080"))
}

func TestWriteCatalog_printsOneRowPerRegisteredMessageSortedByName(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := WriteCatalog(&buf); err != nil {
		t.Fatal(err)
	}
	lines := strings.Split(strings.TrimSuffix(buf.String(), "\n"), "\n")
	if len(lines) != len(registry)+2 || lines[0] != "| Message | Required attrs |" || lines[1] != "| --- | --- |" {
		t.Fatalf("catalog has %d lines, want a header plus %d rows:\n%s", len(lines), len(registry), buf.String())
	}
	rows := lines[2:]
	names := make([]string, len(rows))
	for i, row := range rows {
		names[i] = strings.Split(row, "`")[1]
	}
	if !slices.IsSorted(names) {
		t.Fatalf("catalog rows are not sorted by name:\n%s", buf.String())
	}
	for _, want := range []string{"| `boot.config` | `service`, `config` |", "| `bus.relay.idle` |  |"} {
		if !slices.Contains(rows, want) {
			t.Errorf("catalog lacks row %q", want)
		}
	}
}

func TestWriteCatalog_writeFailureIsInternal(t *testing.T) {
	t.Parallel()
	err := WriteCatalog(failingWriter{})
	if !errors.Is(err, io.ErrClosedPipe) || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("WriteCatalog = %v, want an internal error wrapping io.ErrClosedPipe", err)
	}
}
