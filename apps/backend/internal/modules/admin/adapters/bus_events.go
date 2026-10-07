package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/modules/admin/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
)

type BusEvents struct{ Conn *bus.Conn }

func (b BusEvents) EventAt(ctx context.Context, seq uint64) (app.Event, error) {
	msg, err := b.Conn.EventAt(ctx, seq)
	if err != nil {
		return app.Event{}, err
	}
	return app.Event{Subject: msg.Subject, ID: bus.EventIDOf(msg.Header)}, nil
}
