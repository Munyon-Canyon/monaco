package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

const DisableDevicesDurable = "notify_disable_devices"

func DisableDevicesHandlers() []bus.HandlerSpec {
	return []bus.HandlerSpec{
		bus.Handle("notify.disable_devices_sessions_revoked",
			func(ctx context.Context, tx db.Tx, e events.UserSessionsRevoked, at time.Time) error {
				return app.DisableDevices(ctx, tx, ids.UserIDFrom(e.UserID), at)
			}),
		bus.Handle("notify.disable_devices_user_deleted",
			func(ctx context.Context, tx db.Tx, e events.UserDeleted, at time.Time) error {
				return app.DisableDevices(ctx, tx, ids.UserIDFrom(e.UserID), at)
			}),
	}
}
