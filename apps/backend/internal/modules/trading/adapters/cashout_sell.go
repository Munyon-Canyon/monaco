package adapters

import (
	"context"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/auth"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type CashOutSell struct {
	Sells *app.SellForCashOutHandler
	UoW   *db.UnitOfWork
}

func (c CashOutSell) Handle(ctx context.Context, d bus.Delivery, ev events.CashOutStarted) error {
	ctx = auth.WithActor(ctx, auth.Actor{Kind: auth.ActorSystem, ID: d.Handler})
	err := c.Sells.Handle(ctx, app.SellForCashOut{
		CabalID: ids.CabalIDFrom(ev.CabalID), Source: domain.Source{Kind: domain.SourceCashout, ID: ev.JobID},
		USDCNeeded: ev.SellUSDC,
	}, bus.Heartbeat(ctx))
	if err != nil {
		return err
	}
	return c.UoW.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		_, err := d.Record(ctx, tx)
		return err
	})
}
