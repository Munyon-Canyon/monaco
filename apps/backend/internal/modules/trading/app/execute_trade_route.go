package app

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/trading/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
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

type retryRoute struct {
	stock    multichain.StockID
	routed   bool
	stranded bool
	returned bool
	listing  multichain.ListingID
}

func (h *ExecuteTradeHandler) reroute(ctx context.Context, cmd ExecuteTrade) (ExecuteTrade, error) {
	if cmd.Action != domain.ActionBuy || h.d.Router == nil {
		return cmd, nil
	}
	route, err := h.routedStock(ctx, cmd)
	switch {
	case err != nil || !route.routed && !route.stranded && !route.returned:
		return cmd, err
	case route.stranded || route.returned:
		return h.strandedRoute(ctx, cmd, route)
	}
	return h.freshRoute(ctx, cmd, route)
}

func (h *ExecuteTradeHandler) freshRoute(
	ctx context.Context,
	cmd ExecuteTrade,
	route retryRoute,
) (ExecuteTrade, error) {
	var err error
	if cmd.Mint, err = h.solanaMint(ctx, cmd); err != nil {
		return cmd, err
	}
	fresh, err := h.d.Router.Best(ctx, route.stock, multichain.SideBuy, cmd.USDCMicros)
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

func (h *ExecuteTradeHandler) strandedRoute(
	ctx context.Context,
	cmd ExecuteTrade,
	route retryRoute,
) (ExecuteTrade, error) {
	of := cmd.Retry.Of
	var solana *multichain.Comparison
	if !route.returned {
		fresh, err := h.d.Router.Best(ctx, route.stock, multichain.SideBuy, cmd.USDCMicros)
		if errs.Retryable(errs.CodeOf(err)) {
			return cmd, err
		}
		cash, err := h.d.Routes.Stranded(ctx, of)
		if err != nil {
			return cmd, err
		}
		beats := false
		if solana, beats = solanaBeats(fresh, route.listing, cash); !beats {
			return cmd, nil
		}
	}
	landed, done, err := h.d.Routes.ReturnStranded(ctx, of)
	switch {
	case err != nil:
		return cmd, err
	case !done:
		return cmd, errs.New(errs.CodeUpstreamUnavailable, "trading.ExecuteTrade.strandedRoute",
			slog.String("swap_id", of.String()), slog.String("reason", "bridge_back_pending"))
	}
	if cmd.Mint, err = h.solanaMint(ctx, cmd); err != nil {
		return cmd, err
	}
	cmd.USDCMicros, cmd.Route, cmd.unrouted = landed, solana, solana == nil
	return cmd, nil
}

func solanaBeats(
	fresh multichain.Comparison, stranded multichain.ListingID, cash StrandedCash,
) (*multichain.Comparison, bool) {
	var evm, best *multichain.Quote
	for i := range fresh.Ranked {
		q := &fresh.Ranked[i]
		switch {
		case q.ListingID == stranded:
			evm = q
		case q.Network == chain.NetworkSolana && (best == nil || q.NetShares > best.NetShares):
			best = q
		}
	}
	if best == nil {
		return nil, false
	}
	solana := fresh
	solana.Winner = best.ListingID
	back := sharesFor(best.NetShares, cash.Back, best.InputMicros.Uint64())
	if evm == nil {
		return &solana, true
	}
	swapIn, _ := evm.InputMicros.Sub(money.MicrosFromUint64(costOf(evm.Costs, multichain.CostBridge)))
	return &solana, back > sharesFor(evm.NetShares, cash.OnEVM, swapIn.Uint64())
}

func sharesFor(shares multichain.ShareNanos, cash money.Micros, quotedOn uint64) uint64 {
	n, _ := money.MulDiv(uint64(shares), cash.Uint64(), max(quotedOn, 1))
	return n
}

func costOf(costs []multichain.Cost, kind multichain.CostKind) uint64 {
	total := uint64(0)
	for _, c := range costs {
		if c.Kind == kind {
			total += c.Micros.Uint64()
		}
	}
	return total
}

func (h *ExecuteTradeHandler) routedStock(ctx context.Context, cmd ExecuteTrade) (retryRoute, error) {
	const op = "trading.ExecuteTrade.reroute"
	if cmd.Route != nil || cmd.Retry == nil {
		return retryRoute{stock: routeStock(cmd.Route), routed: cmd.Route != nil}, nil
	}
	q := sqlc.New(h.d.Reads)
	row, err := q.FindTradeRoute(ctx, cmd.Retry.Of.UUID())
	if errors.Is(err, sql.ErrNoRows) {
		return retryRoute{}, nil
	}
	stranded := false
	if err == nil {
		stranded, err = q.StrandedCash(ctx, cmd.Retry.Of.UUID())
	}
	returned := false
	if err == nil {
		_, err = readStep(ctx, q, cmd.Retry.Of, domain.StepBridgeBack)
		returned = err == nil
		if errs.CodeOf(err) == errs.CodeNotFound {
			err = nil
		}
	}
	if err != nil {
		return retryRoute{}, errs.Wrap(err, errs.CodeInternal, op)
	}
	var stored multichain.Comparison
	if err := json.Unmarshal(row.Comparison, &stored); err != nil {
		return retryRoute{}, errs.Wrap(err, errs.CodeDecodeFailed, op)
	}
	return retryRoute{
		stock: stored.StockID, routed: !stranded && !returned, stranded: stranded, returned: returned,
		listing: stored.Winner,
	}, nil
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
