package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
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
	case cmd.Retry == nil || cmd.unrouted:
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

func (h *ExecuteTradeHandler) reroute(ctx context.Context, cmd ExecuteTrade) (ExecuteTrade, error) {
	if cmd.Action != domain.ActionBuy || h.d.Router == nil {
		return cmd, nil
	}
	stock, routed, err := h.routedStock(ctx, cmd)
	if err != nil || !routed {
		return cmd, err
	}
	if cmd.Mint, err = h.solanaMint(ctx, cmd); err != nil {
		return cmd, err
	}
	fresh, err := h.d.Router.Best(ctx, stock, multichain.SideBuy, cmd.USDCMicros)
	switch {
	case err == nil:
		cmd.Route = &fresh
	case errs.Retryable(errs.CodeOf(err)):
		return cmd, err
	default:
		cmd.Route, cmd.unrouted = nil, true
	}
	return cmd, nil
}

func (h *ExecuteTradeHandler) routedStock(ctx context.Context, cmd ExecuteTrade) (multichain.StockID, bool, error) {
	const op = "trading.ExecuteTrade.reroute"
	if cmd.Route != nil || cmd.Retry == nil {
		return routeStock(cmd.Route), cmd.Route != nil, nil
	}
	q := sqlc.New(h.d.Reads)
	row, err := q.FindTradeRoute(ctx, cmd.Retry.Of.UUID())
	if errors.Is(err, sql.ErrNoRows) {
		return multichain.StockID{}, false, nil
	}
	stranded := false
	if err == nil {
		stranded, err = q.StrandedCash(ctx, cmd.Retry.Of.UUID())
	}
	if err != nil {
		return multichain.StockID{}, false, errs.Wrap(err, errs.CodeInternal, op)
	}
	var stored multichain.Comparison
	if err := json.Unmarshal(row.Comparison, &stored); err != nil {
		return multichain.StockID{}, false, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return stored.StockID, !stranded, nil
}

func (h *ExecuteTradeHandler) solanaMint(ctx context.Context, cmd ExecuteTrade) (chain.SolanaAddress, error) {
	if !isEVMAsset(string(cmd.Mint)) {
		return cmd.Mint, nil
	}
	asset, err := h.d.Ports.Catalog.AssetBySymbol(ctx, cmd.Symbol)
	if errs.CodeOf(err) == errs.CodeAssetNotFound {
		return cmd.Mint, nil
	}
	return asset.Mint.Address(), err
}

func routeStock(c *multichain.Comparison) multichain.StockID {
	if c == nil {
		return multichain.StockID{}
	}
	return c.StockID
}
