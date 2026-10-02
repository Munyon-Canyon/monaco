package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const (
	usdcMintAddress = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	usdcDecimals    = 6
	checkRouteOp    = "market.CheckRoute"
)

type Side string

const (
	SideBuy  Side = "buy"
	SideSell Side = "sell"
)

type Quote struct {
	InAmount       money.BaseUnits
	OutAmount      money.BaseUnits
	PriceImpactBps int64
	Routable       bool
}

type Quoter interface {
	Quote(ctx context.Context, in, out domain.Mint, amount money.BaseUnits) (Quote, error)
}

type RouteCheck struct {
	InAmount       money.BaseUnits
	OutAmount      money.BaseUnits
	PriceImpactBps int64
	QuotedAt       time.Time
}

type RouteChecker struct {
	assets      AssetLookup
	quotes      Quoter
	clock       clock.Clock
	usdcAddress string
}

func NewRouteChecker(assets AssetLookup, quotes Quoter, clk clock.Clock) *RouteChecker {
	return &RouteChecker{assets: assets, quotes: quotes, clock: clk, usdcAddress: usdcMintAddress}
}

func (r *RouteChecker) CheckRoute(
	ctx context.Context, id domain.AssetID, side Side, amount money.BaseUnits,
) (RouteCheck, error) {
	asset, err := r.assets.AssetByID(ctx, id)
	if err != nil {
		return RouteCheck{}, err
	}
	if !asset.Tradable() {
		return RouteCheck{}, errs.New(errs.CodeAssetUntradable, checkRouteOp, slog.String("symbol", asset.Symbol))
	}
	if amount.IsZero() {
		return RouteCheck{}, errs.New(errs.CodeInvalidInput, checkRouteOp, slog.String("amount", amount.String()))
	}
	in, out, inDec, outDec, err := r.legs(asset, side)
	if err != nil {
		return RouteCheck{}, err
	}
	if amount.Decimals() != inDec {
		return RouteCheck{}, errs.New(errs.CodeInvalidInput, checkRouteOp,
			slog.String("amount", amount.String()), slog.Int("decimals", int(amount.Decimals())))
	}
	q, err := r.quotes.Quote(ctx, in, out, amount)
	if err != nil {
		return RouteCheck{}, err
	}
	if !q.Routable {
		return RouteCheck{}, errs.New(errs.CodeNoRoute, checkRouteOp,
			slog.String("symbol", asset.Symbol), slog.String("side", string(side)))
	}
	return RouteCheck{
		InAmount:       money.NewBaseUnits(q.InAmount.Uint64(), inDec),
		OutAmount:      money.NewBaseUnits(q.OutAmount.Uint64(), outDec),
		PriceImpactBps: q.PriceImpactBps,
		QuotedAt:       r.clock.Now(),
	}, nil
}

func (r *RouteChecker) legs(asset domain.Asset, side Side) (domain.Mint, domain.Mint, uint8, uint8, error) {
	switch side {
	case SideBuy, SideSell:
	default:
		return domain.Mint{}, domain.Mint{}, 0, 0, errs.New(
			errs.CodeInvalidInput, checkRouteOp, slog.String("side", string(side)))
	}
	usdc, err := domain.ParseMint(r.usdcAddress)
	if err != nil {
		return domain.Mint{}, domain.Mint{}, 0, 0, err
	}
	if side == SideSell {
		return asset.Mint, usdc, asset.Decimals, usdcDecimals, nil
	}
	return usdc, asset.Mint, usdcDecimals, asset.Decimals, nil
}
