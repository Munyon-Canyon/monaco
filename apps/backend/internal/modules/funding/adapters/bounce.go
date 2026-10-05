package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/funding/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type Bounce struct {
	Bouncer *app.Bouncer
	UoW     *db.UnitOfWork
}

func (b Bounce) Handle(ctx context.Context, d bus.Delivery, ev events.CabalExternalDepositDetected) error {
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: d.Handler})
	defer bus.KeepAlive(ctx)()
	if err := b.Bouncer.Start(ctx, ev.ExternalDepositID); err != nil {
		return err
	}
	return b.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := d.Record(ctx, tx)
		return err
	})
}
