package app

import (
	"context"
	"encoding/json"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
)

func RecordRoute(
	ctx context.Context, tx db.Tx, swap ids.SwapID, c multichain.Comparison, pin chain.Network, at time.Time,
) error {
	body, err := json.Marshal(c)
	if err == nil {
		err = sqlc.New(tx.Queries()).InsertTradeRoute(ctx, sqlc.InsertTradeRouteParams{
			TradeID: swap.UUID(), Comparison: body, WinnerListingID: c.Winner.UUID(), CreatedAt: at,
			PinnedNetwork: string(pin),
		})
	}
	if err != nil {
		return errs.Wrap(err, errs.CodeInternal, "trading.RecordRoute")
	}
	return nil
}
