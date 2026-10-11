package app

import (
	"context"
	"database/sql"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
)

func (h *ExecuteTradeHandler) winner(ctx context.Context, cmd ExecuteTrade) (multichain.Listing, error) {
	id, routed, err := h.winnerID(ctx, cmd)
	if err != nil || !routed {
		return multichain.Listing{}, err
	}
	listing, err := h.d.Listings.Listing(ctx, id)
	if listing.Network.IsEVM() && !h.d.EVM {
		return multichain.Listing{}, err
	}
	return listing, err
}

func (h *ExecuteTradeHandler) winnerID(ctx context.Context, cmd ExecuteTrade) (multichain.ListingID, bool, error) {
	switch {
	case cmd.Route != nil:
		return cmd.Route.Winner, true, nil
	case cmd.Retry == nil:
		return multichain.ListingID{}, false, nil
	}
	row, err := sqlc.New(h.d.Reads).FindTradeRoute(ctx, cmd.Retry.Of.UUID())
	switch {
	case errors.Is(err, sql.ErrNoRows):
		return multichain.ListingID{}, false, nil
	case err != nil:
		return multichain.ListingID{}, false, errs.Wrap(err, errs.CodeInternal, "trading.ExecuteTrade.winner")
	}
	return multichain.ListingIDFrom(row.WinnerListingID), true, nil
}
