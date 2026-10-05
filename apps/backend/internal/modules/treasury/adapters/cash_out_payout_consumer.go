package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
)

type CashOutPayout struct {
	Payouts *app.CashOutPayouts
	UoW     *db.UnitOfWork
}

func (c CashOutPayout) Handle(ctx context.Context, d bus.Delivery, ev events.CashOutStarted) error {
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: d.Handler})
	defer bus.KeepAlive(ctx)()
	if err := c.Payouts.Advance(ctx, ev.JobID, app.CashOutPayoutWait, nil); err != nil {
		return err
	}
	return c.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := d.Record(ctx, tx)
		return err
	})
}
