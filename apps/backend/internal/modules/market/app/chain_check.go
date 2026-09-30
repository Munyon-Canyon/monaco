package app

import (
	"context"
	"errors"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	chainChecksPerTick    = 200
	chainCheckConcurrency = 8
)

type chainCheck struct {
	asset      domain.Asset
	decimals   uint8
	multiplier domain.Multiplier
	err        error
}

func (p *CatalogPoller) checkChainFacts(ctx context.Context) (int, error) {
	const op = "market.CatalogPoller.checkChainFacts"
	rows, err := sqlc.New(p.reads).UncheckedAssets(ctx, chainChecksPerTick)
	unchecked, err := many(rows, err, op)
	if err != nil {
		return 0, err
	}
	checks, err := concurrency.FanOut(ctx, chainCheckConcurrency, unchecked, p.askChain)
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), op)
	}
	return p.storeChainFacts(ctx, checks)
}

func (p *CatalogPoller) askChain(ctx context.Context, a domain.Asset) (chainCheck, error) {
	c := chainCheck{asset: a}
	var num, den uint64
	if c.decimals, num, den, c.err = p.facts.Facts(ctx, a.Mint); c.err == nil {
		c.multiplier, c.err = domain.NewMultiplier(num, den)
	}
	if c.err != nil {
		c.err = errs.Wrap(c.err, errs.CodeOf(c.err), "market.CatalogPoller.askChain", slog.String("symbol", a.Symbol))
	}
	return c, nil
}

func (p *CatalogPoller) storeChainFacts(ctx context.Context, checks []chainCheck) (int, error) {
	params := sqlc.StoreChainFactsParams{Now: p.clock.Now()}
	answered := map[string]chainCheck{}
	var failed []error
	for _, c := range checks {
		if c.err != nil {
			failed = append(failed, c.err)
			continue
		}
		mint := c.asset.Mint.String()
		answered[mint] = c
		params.Mints = append(params.Mints, mint)
		params.IssuerDecimals = append(params.IssuerDecimals, int16(c.asset.Decimals))
		params.ChainDecimals = append(params.ChainDecimals, int16(c.decimals))
		params.MultiplierNums = append(params.MultiplierNums, c.multiplier.Num)
		params.MultiplierDens = append(params.MultiplierDens, c.multiplier.Den)
	}
	if len(params.Mints) == 0 {
		return 0, errors.Join(failed...)
	}
	var stored []string
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		if stored, err = sqlc.New(tx.Queries()).StoreChainFacts(ctx, params); err != nil {
			return err
		}
		tx.AfterCommit(func(ctx context.Context) { logCorrections(ctx, stored, answered) })
		return nil
	})
	if err != nil {
		failed = append(failed, errs.Wrap(err, errs.CodeOf(err), "market.CatalogPoller.storeChainFacts"))
		return 0, errors.Join(failed...)
	}
	return len(stored), errors.Join(failed...)
}

func logCorrections(ctx context.Context, stored []string, answered map[string]chainCheck) {
	for _, mint := range stored {
		c := answered[mint]
		if c.decimals == c.asset.Decimals {
			continue
		}
		observability.Degraded(ctx, observability.MarketDecimalsCorrected, slog.String("symbol", c.asset.Symbol),
			slog.String("mint", mint), slog.Int("issuer_decimals", int(c.asset.Decimals)),
			slog.Int("chain_decimals", int(c.decimals)))
	}
}
