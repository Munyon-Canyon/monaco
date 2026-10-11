package app

import (
	"cmp"
	"context"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type TradePreview struct {
	QuoteOut        uint64
	Advisory        errs.Code
	Pot             money.Micros
	PriceComparison *multichain.Comparison
}

func (h *ProposeTradeHandler) Preview(ctx context.Context, req ProposeTrade) (TradePreview, error) {
	if err := member(ctx, h.ports.Cabals, req.CabalID, req.ProposerID); err != nil {
		return TradePreview{}, err
	}
	pot, err := h.ports.Treasury.PotValue(ctx, req.CabalID)
	if err != nil {
		return TradePreview{}, err
	}
	out := TradePreview{Pot: pot}
	asset, err := h.ports.Assets.AssetBySymbol(ctx, req.Trade.Symbol)
	if err != nil {
		return out, advise(&out, err)
	}
	fundsErr := funds(ctx, h.ports, req.CabalID, asset, req.Trade, pot)
	quote, routeErr := route(ctx, h.ports.Routes, asset, req.Trade)
	if market.ProbeFailed(routeErr) {
		routeErr = errs.New(errs.CodeNoRoute, "governance.Preview")
	}
	var pinErr error
	if routeErr == nil {
		out.PriceComparison = h.compare(ctx, asset, req.Trade)
		var priced *multichain.Comparison
		priced, pinErr = pinned(out.PriceComparison, req.Trade.Network)
		out.QuoteOut = displayQuote(asset, priced, quote.OutAmount.Uint64(), h.clock.Now())
	}
	failed := slices.DeleteFunc([]error{fundsErr, routeErr, pinErr}, func(err error) bool { return err == nil })
	for _, err := range failed {
		if err := advise(&out, err); err != nil {
			return TradePreview{}, err
		}
	}
	return out, nil
}

func advise(p *TradePreview, err error) error {
	code := errs.CodeOf(err)
	if !slices.Contains([]errs.Code{
		errs.CodeAssetNotFound, errs.CodeAssetUntradable, errs.CodeAssetPaused, errs.CodeNoRoute,
		errs.CodePotExceeded, errs.CodeInsufficientFunds, errs.CodePotCashShort, errs.CodeCabalSharesShort,
		errs.CodeInvalidNetwork,
	}, code) {
		return err
	}
	p.Advisory = cmp.Or(p.Advisory, code)
	return nil
}
