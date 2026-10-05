package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const cashOutSource = "cashout"

type CashOut struct {
	Sales app.CashOutSales
}

func (c CashOut) Started(ctx context.Context, tx db.Tx, e events.CashOutStarted, at time.Time) error {
	return c.Sales.Started(ctx, tx, e, at)
}

func (c CashOut) Confirmed(ctx context.Context, tx db.Tx, e events.TradeConfirmed, at time.Time) error {
	if e.Source.Kind != cashOutSource {
		return nil
	}
	return c.Sales.Result(ctx, tx, app.SaleResult{
		JobID: e.Source.ID, SwapID: e.SwapID, CabalID: ids.CabalIDFrom(e.CabalID), BatchSize: e.SourceBatchSize,
		Confirmed: true, USDCOut: money.MicrosFromUint64(e.OutAmount),
	}, at)
}

func (c CashOut) Failed(ctx context.Context, tx db.Tx, e events.TradeFailed, at time.Time) error {
	if e.Source.Kind != cashOutSource {
		return nil
	}
	return c.Sales.Result(ctx, tx, app.SaleResult{
		JobID: e.Source.ID, SwapID: e.SwapID, CabalID: ids.CabalIDFrom(e.CabalID), BatchSize: e.SourceBatchSize,
	}, at)
}
