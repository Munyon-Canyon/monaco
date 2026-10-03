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
}

func (h Trades) Handle(ctx context.Context, tx db.Tx, e events.TradeConfirmed, at time.Time) error {
	return h.Ledger.At(at).PostSwap(ctx, tx, domain.Swap{
		ID:          e.SwapID,
		CabalID:     ids.CabalIDFrom(e.CabalID),
		In:          domain.Leg{Asset: domain.MintAsset(e.InMint), Amount: e.InAmount},
		Out:         domain.Leg{Asset: domain.MintAsset(e.OutMint), Amount: e.OutAmount},
		Fee:         e.FeeMicros,
		TxSignature: e.TxSignature,
	})
}
