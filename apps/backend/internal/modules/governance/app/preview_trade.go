package app

import (
	"cmp"
	"context"
	"slices"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/governance/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market"
	"github.com/monaco/monaco/apps/backend/internal/platform/chain/multichain"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type TradePreview struct {
	QuoteOut        uint64
	Advisory        errs.Code
	Pot             money.Micros
	PriceComparison *multichain.Comparison
	Spendable       *Spendable
}

type Spendable struct {
	Cash   money.Micros
	MaxBuy money.Micros
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
	if req.Trade.Kind == domain.KindBuy {
		cash, cfg, err := spendableCash(ctx, h.ports, req.CabalID, asset)
		if err != nil {
			return TradePreview{}, err
		}
		out.Spendable = &Spendable{Cash: cash, MaxBuy: money.MicrosFromUint64(cfg.MaxSpend(cash.Uint64()))}
		if req.Trade.USDCMicros.Uint64() == 0 {
			return out, nil
		}
	}
	if req.Trade.Kind == domain.KindSell {
		return h.previewSell(ctx, req, asset, out)
	}
	fundsErr := funds(ctx, h.ports, req.CabalID, asset, req.Trade, pot)
	routed, comparison, routeErr := h.route(ctx, asset, req.Trade)
	if market.ProbeFailed(routeErr) {
		routeErr = errs.New(errs.CodeNoRoute, "governance.Preview")
	}
	var pinErr error
	if routeErr == nil {
		out.PriceComparison = comparison
		var priced *multichain.Comparison
		priced, pinErr = pinned(comparison, req.Trade.Network)
		out.QuoteOut = displayQuote(asset, priced, routed, h.clock.Now())
	}
	failed := slices.DeleteFunc([]error{fundsErr, routeErr, pinErr}, func(err error) bool { return err == nil })
	for _, err := range failed {
		if err := advise(&out, err); err != nil {
			return TradePreview{}, err
		}
	}
	return out, nil
}

func (h *ProposeTradeHandler) previewSell(
	ctx context.Context, req ProposeTrade, asset market.Asset, out TradePreview,
) (TradePreview, error) {
	target, err := h.sellTarget(ctx, req.CabalID, asset, req.Trade)
	if err != nil {
		return out, advise(&out, err)
	}
	fundsErr := h.sellFunds(ctx, req.CabalID, asset, req.Trade, target)
	a, routeErr := h.sellRoute(ctx, asset, req.Trade, target)
	if market.ProbeFailed(routeErr) {
		routeErr = errs.New(errs.CodeNoRoute, "governance.Preview")
	}
	if routeErr == nil {
		out.QuoteOut = a.routed
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
		errs.CodeAssetNotFound, errs.CodeAssetUntradable, errs.CodeAssetPaused, errs.CodeNoRoute,
		errs.CodePotExceeded, errs.CodeInsufficientFunds, errs.CodePotCashShort, errs.CodeOverTradeCap,
		errs.CodeCabalSharesShort, errs.CodeInvalidNetwork, errs.CodeQuoteUnavailable,
	}, code) {
		return err
	}
	p.Advisory = cmp.Or(p.Advisory, code)
	return nil
}
