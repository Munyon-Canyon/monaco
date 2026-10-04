package app

import (
	"cmp"
	"context"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type TradePreview struct {
	QuoteOut uint64
	Advisory errs.Code
	Pot      money.Micros
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
	fundsErr := funds(ctx, h.ports.Treasury, req.CabalID, asset, req.Trade, pot)
	quote, routeErr := route(ctx, h.ports.Routes, asset, req.Trade)
	if routeErr == nil {
		out.QuoteOut = quote.OutAmount.Uint64()
	}
	failed := slices.DeleteFunc([]error{fundsErr, routeErr}, func(err error) bool { return err == nil })
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
		errs.CodeAssetNotFound, errs.CodeAssetUntradable, errs.CodeNoRoute, errs.CodePotExceeded,
		errs.CodeInsufficientFunds,
	}, code) {
		return err
	}
	p.Advisory = cmp.Or(p.Advisory, code)
	return nil
}
