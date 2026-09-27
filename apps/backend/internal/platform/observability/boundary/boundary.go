package boundary

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/internal/emit"
)

func Warn(ctx context.Context, m observability.Msg, attrs ...slog.Attr) {
	emit.Log(ctx, slog.LevelWarn, m.Name, attrs)
}

func Error(ctx context.Context, m observability.Msg, attrs ...slog.Attr) {
	emit.Log(ctx, slog.LevelError, m.Name, attrs)
}

func Stopped(ctx context.Context, service string, err error) {
	Error(ctx, observability.BootStopped, slog.String("service", service),
		slog.String("code", string(errs.CodeOf(err))), slog.Any("err", err),
		slog.GroupAttrs("detail", errs.Detail(err)...))
}
