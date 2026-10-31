package app

import (
	"context"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func TestAFirstReadBlipIsRetryableForTheTradeAndTheCashOutSale(t *testing.T) {
	t.Parallel()
	reads := testkit.DB(t)
	gen := testkit.NewIDs(1)
	gone, cancel := context.WithCancel(t.Context())
	cancel()
	event := ids.EventIDFrom(gen.NewV7())

	engine := NewExecuteTradeHandler(ExecuteTradeDeps{Reads: reads})
	err := engine.Handle(gone, bus.Delivery{Handler: "trading.engine", EventID: event},
		ExecuteTrade{Action: domain.ActionBuy}, nil)
	if code := db.CodeOf(err); err == nil || errs.VerdictFor(code) != errs.VerdictNak {
		t.Fatalf("trading.engine: err %v, code %s; want an error that naks", err, code)
	}

	sells := NewSellForCashOutHandler(SellForCashOutDeps{Reads: reads})
	err = sells.HandleDelivery(gone, bus.Delivery{Handler: "trading.cashout_sell", EventID: event},
		SellForCashOut{Source: domain.Source{Kind: domain.SourceCashout, ID: gen.NewV7()}}, nil)
	if code := db.CodeOf(err); err == nil || errs.VerdictFor(code) != errs.VerdictNak {
		t.Fatalf("trading.cashout_sell: err %v, code %s; want an error that naks", err, code)
	}
}
