package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	usdcMintAddress = "EPjFWdd5AufqSSqeM2qN1xzybapC8G4wEGGkZwyTDt1v"
	usdcDecimals    = 6
	probeUSDC       = 1_000_000
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

type SampledPrices interface {
	PricesAsOf(ctx context.Context, ids []domain.AssetID, at time.Time) (map[domain.AssetID]domain.Sample, error)
}

type probeError struct{ error }

func (e probeError) Unwrap() error { return e.error }

func ProbeFailure(err error) error { return probeError{err} }

func ProbeFailed(err error) bool {
	var p probeError
	return errors.As(err, &p)
}

type RouteChecker struct {
	assets      AssetLookup
	prices      SampledPrices
	quotes      Quoter
	clock       clock.Clock
	usdcAddress string
}

func NewRouteChecker(assets AssetLookup, prices SampledPrices, quotes Quoter, clk clock.Clock) *RouteChecker {
	return &RouteChecker{assets: assets, prices: prices, quotes: quotes, clock: clk, usdcAddress: usdcMintAddress}
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
		return RouteCheck{}, r.unrouted(ctx, asset, side, amount, in, out)
	}
	return RouteCheck{
		InAmount:       money.NewBaseUnits(q.InAmount.Uint64(), inDec),
		OutAmount:      money.NewBaseUnits(q.OutAmount.Uint64(), outDec),
		PriceImpactBps: q.PriceImpactBps,
		QuotedAt:       r.clock.Now(),
	}, nil
}

func (r *RouteChecker) unrouted(
	ctx context.Context, asset domain.Asset, side Side, amount money.BaseUnits, in, out domain.Mint,
) error {
	probe := money.NewBaseUnits(probeUSDC, usdcDecimals)
	if side == SideSell {
		probe = money.NewBaseUnits(min(amount.Uint64(), r.sellProbe(ctx, asset)), asset.Decimals)
	}
	attrs := []slog.Attr{slog.String("symbol", asset.Symbol), slog.String("side", string(side))}
	if amount.Uint64() <= probe.Uint64() {
		return errs.New(errs.CodeNoRoute, checkRouteOp, attrs...)
	}
	q, err := r.quotes.Quote(ctx, in, out, probe)
	switch {
	case err != nil:
		observability.Degraded(ctx, observability.MarketRouteProbeFailed,
			slog.String("symbol", asset.Symbol), slog.String("side", string(side)),
			slog.String("error", err.Error()), slog.String("code", string(errs.CodeOf(err))))
		return probeError{err}
	case q.Routable:
		return errs.New(errs.CodeNoRoute, checkRouteOp, attrs...)
	}
	return errs.New(errs.CodeAssetPaused, checkRouteOp, attrs...)
}

func (r *RouteChecker) sellProbe(ctx context.Context, asset domain.Asset) uint64 {
	whole := money.OneWhole(asset.Decimals)
	samples, err := r.prices.PricesAsOf(ctx, []domain.AssetID{asset.ID}, r.clock.Now())
	if err != nil {
		return whole
	}
	sample, ok := samples[asset.ID]
	if !ok || sample.Micros.IsZero() {
		return whole
	}
	m := asset.UIMultiplierAt(r.clock.Now())
	if m.Num <= 0 || m.Den <= 0 {
		return whole
	}
	shares, err := money.MulDiv(whole, uint64(m.Den), uint64(m.Num))
	if err != nil {
		return whole
	}
	worth, err := money.MulDiv(shares, probeUSDC, sample.Micros.Uint64())
	if err != nil || worth == 0 {
		return whole
	}
	return worth
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
