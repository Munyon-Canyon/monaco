package observability

import (
	"bytes"
	"errors"
	"io"
	"log/slog"
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

func TestInfo_withoutLoggerInContextWritesNothing(t *testing.T) {
	t.Parallel()
	Info(t.Context(), BootListening, slog.String("service", "api"), slog.String("addr", ":8080"))
}

func TestWriteCatalog_printsTheRegistrySortedByName(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	if err := WriteCatalog(&buf); err != nil {
		t.Fatal(err)
	}
	want := "| Message | Required attrs |\n| --- | --- |\n" +
		"| `boot.config` | `service`, `config` |\n" +
		"| `boot.listening` | `service`, `addr` |\n" +
		"| `boot.stopped` | `service`, `err` |\n" +
		"| `db.lock.lost` | `lock`, `held`, `err` |\n" +
		"| `http.problem` | `code`, `status`, `err`, `alert` |\n" +
		"| `http.request` | `method`, `route`, `status`, `duration_ms` |\n" +
		"| `httpclient.retry` | `upstream`, `attempt`, `status`, `delay` |\n" +
		"| `tx.committed` | `event_ids`, `attempt` |\n" +
		"| `tx.retry` | `code`, `attempt`, `delay` |\n" +
		"| `tx.rolled_back` | `code`, `attempt` |\n"
	if buf.String() != want {
		t.Fatalf("catalog =\n%s\nwant\n%s", buf.String(), want)
	}
}

func TestWriteCatalog_writeFailureIsInternal(t *testing.T) {
	t.Parallel()
	err := WriteCatalog(failingWriter{})
	if !errors.Is(err, io.ErrClosedPipe) || errs.CodeOf(err) != errs.CodeInternal {
		t.Fatalf("WriteCatalog = %v, want an internal error wrapping io.ErrClosedPipe", err)
	}
}
