package boundary

import (
	"bytes"
	"errors"
	"fmt"
	"log/slog"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

func TestWarnAndError_logAtTheirLevelWithRedaction(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	logger := observability.NewLogger(config.Config{Env: config.EnvProduction}, &buf)
	ctx := observability.WithLogger(observability.WithActor(t.Context(), "system"), logger)
	Warn(ctx, observability.BootListening, slog.String("service", "api"), slog.String("addr", ":8080"))
	Error(ctx, observability.BootStopped, slog.String("service", "api"), slog.Any("err", errors.New("boom")),
		slog.String("token", "t0k"))

	got := strings.Split(strings.TrimSpace(buf.String()), "\n")
	want := []string{
		`"level":"WARN","msg":"boot.listening","actor":"system","service":"api","addr":":8080"}`,
		`"level":"ERROR","msg":"boot.stopped","actor":"system","service":"api","err":"boom","token":"***"}`,
	}
	if len(got) != len(want) {
		t.Fatalf("got %d lines, want %d:\n%s", len(got), len(want), buf.String())
	}
	for i := range want {
		if !strings.HasSuffix(got[i], want[i]) {
			t.Errorf("line %d = %s, want suffix %s", i, got[i], want[i])
		}
	}
}

func TestStopped_logsTheCodeAndEveryErrsAttrOfTheBootError(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	ctx := observability.WithLogger(t.Context(), observability.NewLogger(config.Config{}, &buf))
	err := fmt.Errorf("boot: %w", errs.New(errs.CodeDBSchemaBehind, "db.Open",
		slog.String("have", "0001"), slog.String("want", "0002"), slog.String("hint", "run: just migrate db")))

	Stopped(ctx, "api", err)

	want := `"level":"ERROR","msg":"boot.stopped","service":"api","code":"db_schema_behind",` +
		`"err":"boot: db.Open: db_schema_behind",` +
		`"detail":{"have":"0001","want":"0002","hint":"run: just migrate db"}}`
	if got := strings.TrimSpace(buf.String()); !strings.HasSuffix(got, want) {
		t.Fatalf("line = %s, want suffix %s", got, want)
	}
}
