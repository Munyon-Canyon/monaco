package adapters

import (
	"context"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
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
	if e.Source.Kind != cashOutSource || bridgesBack(e.OutMint) {
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

func (c CashOut) Blocked(ctx context.Context, tx db.Tx, e events.TradeBlocked, at time.Time) error {
	if e.Source.Kind != cashOutSource || e.SourceBatchSize != 0 {
		return nil
	}
	return c.Sales.NoLegs(ctx, tx, ids.CabalIDFrom(e.CabalID), e.Source.ID, at)
}

func bridgesBack(out chain.SolanaAddress) bool {
	network, _, found := strings.Cut(string(out), ":")
	return found && chain.Network(network).IsEVM()
}

func (c CashOut) BridgedBack(ctx context.Context, tx db.Tx, e events.TradeStepChanged, at time.Time) error {
	if e.Source.Kind != cashOutSource || e.Step != string(domain.BridgeBack) || e.Attempt > 1 ||
		e.Status != "succeeded" && e.Status != "failed" {
		return nil
	}
	r := app.SaleResult{
		JobID: e.Source.ID, SwapID: e.SwapID, CabalID: ids.CabalIDFrom(e.CabalID), BatchSize: e.SourceBatchSize,
	}
	if e.Status == "succeeded" {
		landed, err := e.AmountMicros.Sub(e.FeeMicros)
		if err != nil {
			return err
		}
		r.Confirmed, r.USDCOut = true, landed
	}
	return c.Sales.Result(ctx, tx, r, at)
}
