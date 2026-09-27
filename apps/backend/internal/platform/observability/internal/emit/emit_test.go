package emit

import (
	"bytes"
	"log/slog"
	"testing"
)

func TestLog_writesThroughTheContextLogger(t *testing.T) {
	t.Parallel()
	var buf bytes.Buffer
	ctx := WithLogger(t.Context(), slog.New(slog.NewTextHandler(&buf, &slog.HandlerOptions{
		ReplaceAttr: func(_ []string, a slog.Attr) slog.Attr {
			if a.Key == slog.TimeKey {
				return slog.Attr{}
			}
			return a
		},
	})))
	Log(ctx, slog.LevelWarn, "poller.tick", []slog.Attr{slog.Int("found", 0)})
	if want := "level=WARN msg=poller.tick found=0\n"; buf.String() != want {
		t.Fatalf("line = %q, want %q", buf.String(), want)
	}
}

func TestLog_withoutLoggerWritesNothingAndDoesNotPanic(t *testing.T) {
	t.Parallel()
	Log(t.Context(), slog.LevelError, "boot.stopped", nil)
}
