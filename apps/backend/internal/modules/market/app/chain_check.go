package app

import (
	"context"
	"fmt"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	chainChecksPerTick = 200
	chainRecheckAge    = 24 * time.Hour
)

type chainCheck struct {
	asset      domain.Asset
	checkedAt  time.Time
	decimals   uint8
	multiplier domain.Multiplier
	next       domain.MultiplierStep
	err        error
}

func (p *CatalogPoller) checkChainFacts(ctx context.Context) (int, error) {
	const op = "market.CatalogPoller.checkChainFacts"
	rows, err := sqlc.New(p.reads).AssetsDueForChainCheck(ctx, sqlc.AssetsDueForChainCheckParams{
		StaleBefore: p.clock.Now().Add(-chainRecheckAge), MaxAssets: chainChecksPerTick,
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), op)
	}
	checks := make([]chainCheck, len(rows))
	mints := make([]domain.Mint, len(rows))
	for i, row := range rows {
		if checks[i].asset, err = toAsset(row); err != nil {
			return 0, err
		}
		checks[i].checkedAt = row.ChainCheckedAt.Time
		mints[i] = checks[i].asset.Mint
	}
	facts, failures, err := p.facts.Facts(ctx, mints)
	for i := range checks {
		c := &checks[i]
		if failure := failures[c.asset.Mint]; failure != nil {
			c.err = errs.Wrap(failure, errs.CodeOf(failure), op, slog.String("symbol", c.asset.Symbol))
			continue
		}
		fact, ok := facts[c.asset.Mint]
		if !ok {
			if err != nil {
				c.err = errs.Wrap(err, errs.CodeOf(err), op, slog.String("symbol", c.asset.Symbol))
			} else {
				c.err = errs.New(errs.CodeDecodeFailed, op, slog.String("symbol", c.asset.Symbol))
			}
			continue
		}
		c.decimals = fact.Decimals
		c.multiplier, c.next, c.err = multipliersOf(fact)
	}
	return p.storeChainFacts(ctx, checks)
}

func multipliersOf(fact MintFact) (domain.Multiplier, domain.MultiplierStep, error) {
	current, err := domain.NewMultiplier(fact.MultiplierNum, fact.MultiplierDen)
	if err != nil || fact.NextMultiplierNum == 0 {
		return current, domain.MultiplierStep{}, err
	}
	next, err := domain.NewMultiplier(fact.NextMultiplierNum, fact.NextMultiplierDen)
	return current, domain.MultiplierStep{To: next, At: fact.NextMultiplierAt}, err
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
		params.NextNums = append(params.NextNums, c.next.To.Num)
		params.NextDens = append(params.NextDens, c.next.To.Den)
		params.NextAts = append(params.NextAts, c.next.At)
		params.Rechecks = append(params.Rechecks, c.asset.ChainChecked)
		params.CheckedAts = append(params.CheckedAts, c.checkedAt)
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
		tx.AfterCommit(func(ctx context.Context) { logChanges(ctx, stored, answered) })
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

func logChanges(ctx context.Context, stored []string, answered map[string]chainCheck) {
	for _, mint := range stored {
		c := answered[mint]
		if c.decimals != c.asset.Decimals {
			observability.Degraded(ctx, observability.MarketDecimalsCorrected, slog.String("symbol", c.asset.Symbol),
				slog.String("mint", mint), slog.Int("issuer_decimals", int(c.asset.Decimals)),
				slog.Int("chain_decimals", int(c.decimals)))
		}
		if c.asset.ChainChecked && (c.multiplier != c.asset.UIMultiplier || !c.next.Equal(c.asset.NextUIMultiplier)) {
			observability.Info(ctx, observability.MarketMultiplierChanged, slog.String("symbol", c.asset.Symbol),
				slog.String("mint", mint),
				slog.String("multiplier_before", schedule(c.asset.UIMultiplier, c.asset.NextUIMultiplier)),
				slog.String("multiplier_after", schedule(c.multiplier, c.next)))
		}
	}
}

func schedule(current domain.Multiplier, next domain.MultiplierStep) string {
	if !next.Scheduled() {
		return fmt.Sprintf("%d/%d", current.Num, current.Den)
	}
	return fmt.Sprintf("%d/%d then %d/%d at %s", current.Num, current.Den, next.To.Num, next.To.Den,
		next.At.UTC().Format(time.RFC3339))
}
