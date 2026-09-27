package emit

import (
	"context"
	"log/slog"
)

type loggerKey struct{}

func WithLogger(ctx context.Context, l *slog.Logger) context.Context {
	return context.WithValue(ctx, loggerKey{}, l)
}

func Log(ctx context.Context, level slog.Level, name string, attrs []slog.Attr) {
	if l, ok := ctx.Value(loggerKey{}).(*slog.Logger); ok {
		l.LogAttrs(ctx, level, name, attrs...)
	}
}
