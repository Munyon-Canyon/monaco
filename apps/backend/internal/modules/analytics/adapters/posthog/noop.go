package posthog

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type Noop struct{}

func (Noop) Capture(ctx context.Context, batch []app.Capture) error {
	for _, c := range batch {
		observability.Info(ctx, observability.AnalyticsCaptureSkipped,
			slog.String("reason", "no_api_key"), slog.String("event", c.Event), slog.String("uuid", c.UUID.String()))
	}
	return nil
}
