package adapters

import (
	"context"
	"strings"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/notify/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
)

func Push[E events.Event](p *app.Pusher, kinds ...app.Kind[E]) bus.HandlerSpec {
	var zero E
	name := "notify." + strings.ReplaceAll(string(zero.Type()), ".", "_")
	return bus.HandleOwn(name, func(ctx context.Context, d bus.Delivery, e E) error {
		defer bus.KeepAlive(ctx)()
		return app.Notify[E]{Pusher: p, Kinds: kinds}.Handle(ctx, d, e)
	})
}
