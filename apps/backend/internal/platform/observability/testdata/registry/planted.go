package planted

import (
	"context"
	"log/slog"

	obs "github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability/boundary"
)

func calls(ctx context.Context, m obs.Msg) {
	obs.Info(ctx, obs.BootListening, slog.String("service", "api"), slog.String("addr", ":8080"))
	obs.Info(ctx, obs.Msg{Name: "treasury.fund.rejected"})
	obs.Debug(ctx, obs.NotRegistered)
	obs.Info(ctx, m)
	boundary.Error(ctx, obs.BootStopped, slog.String("service", "api"))
	boundary.Warn(ctx, obs.BootConfig, slog.Any("config", nil), slog.String("service", "worker"))
	attrs := []slog.Attr{slog.String("addr", ":1")}
	obs.Info(ctx, obs.BootListening, append(attrs, slog.String("service", "api"))...)
}
