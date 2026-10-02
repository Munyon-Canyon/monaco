package app

import (
	"context"
	"log/slog"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	chainChecksPerTick = 200
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
	mints := make([]domain.Mint, len(unchecked))
	for i, asset := range unchecked {
		mints[i] = asset.Mint
	}
	facts, failures, err := p.facts.Facts(ctx, mints)
	checks := make([]chainCheck, len(unchecked))
	for i, asset := range unchecked {
		checks[i].asset = asset
		if failure := failures[asset.Mint]; failure != nil {
			checks[i].err = errs.Wrap(failure, errs.CodeOf(failure), op, slog.String("symbol", asset.Symbol))
			continue
		}
		fact, ok := facts[asset.Mint]
		if !ok {
			if err != nil {
				checks[i].err = errs.Wrap(err, errs.CodeOf(err), op, slog.String("symbol", asset.Symbol))
			} else {
				checks[i].err = errs.New(errs.CodeDecodeFailed, op, slog.String("symbol", asset.Symbol))
			}
			continue
		}
		checks[i].decimals = fact.Decimals
		checks[i].multiplier, checks[i].err = domain.NewMultiplier(fact.MultiplierNum, fact.MultiplierDen)
	}
	return p.storeChainFacts(ctx, checks)
}

func (p *CatalogPoller) storeChainFacts(ctx context.Context, checks []chainCheck) (int, error) {
	params := sqlc.StoreChainFactsParams{Now: p.clock.Now()}
	answered := map[string]chainCheck{}
	var first error
	failed := 0
	firstMint := ""
	for _, c := range checks {
		if c.err != nil {
			failed++
			if first == nil {
				first, firstMint = c.err, c.asset.Mint.String()
			}
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
		return 0, chainCheckError(first, failed, 0, firstMint)
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
		return 0, errs.Wrap(err, errs.CodeOf(err), "market.CatalogPoller.storeChainFacts")
	}
	return len(stored), chainCheckError(first, failed, len(stored), firstMint)
}

func chainCheckError(first error, failed, checked int, firstMint string) error {
	if first == nil {
		return nil
	}
	return errs.Wrap(first, errs.CodeOf(first), "market.CatalogPoller.storeChainFacts", slog.Int("failed", failed),
		slog.Int("checked", checked), slog.String("first_mint", firstMint))
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
