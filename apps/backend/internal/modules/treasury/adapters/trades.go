package adapters

import (
	"context"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/app"
	"github.com/monaco/monaco/apps/backend/internal/modules/treasury/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

type Trades struct {
	Ledger app.Ledger
	Hints  Hints
}

func (h Trades) Handle(ctx context.Context, tx db.Tx, e events.TradeConfirmed, at time.Time) error {
	err := h.Ledger.At(at).PostSwap(ctx, tx, domain.Swap{
		ID:          e.SwapID,
		CabalID:     ids.CabalIDFrom(e.CabalID),
		In:          domain.Leg{Asset: domain.MintAsset(e.InMint), Amount: e.InAmount},
		Out:         domain.Leg{Asset: domain.MintAsset(e.OutMint), Amount: e.OutAmount},
		Fee:         e.FeeMicros,
		TxSignature: e.TxSignature,
	})
	if err != nil {
		return err
	}
	tx.AfterCommit(func(ctx context.Context) {
		h.Hints.PublishHint(ctx, events.CabalActivityChangedHint(ids.CabalIDFrom(e.CabalID)), nil)
	})
	return nil
}
