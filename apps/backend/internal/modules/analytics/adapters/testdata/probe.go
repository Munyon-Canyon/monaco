package testdata

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics"
	"github.com/monaco/monaco/apps/backend/internal/modules/analytics/app"
)

const ProbeEvent = "probe_fired"

func Register(r *analytics.Registry) {
	analytics.Export(r, string(events.TypeSystemPinged), probe)
}

func probe(_ context.Context, e events.SystemPinged) (app.Capture, bool, error) {
	return app.Capture{Event: ProbeEvent, Properties: map[string]any{"ping_id": e.PingID.String()}}, true, nil
}
