package app

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/concurrency"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

const CatalogInterval = time.Hour

type CatalogPoller struct {
	uow       *db.UnitOfWork
	reads     sqlc.DBTX
	ids       ids.Generator
	clock     clock.Clock
	providers Providers
	facts     MintFacts
}

func NewCatalogPoller(
	uow *db.UnitOfWork, reads sqlc.DBTX, g ids.Generator, c clock.Clock, providers Providers, facts MintFacts,
) *CatalogPoller {
	return &CatalogPoller{uow: uow, reads: reads, ids: g, clock: c, providers: providers, facts: facts}
}

func (*CatalogPoller) Name() string { return "market.catalog" }

func (*CatalogPoller) Interval() time.Duration { return CatalogInterval }

type issuerCatalog struct {
	issuer domain.Issuer
	assets []ProviderAsset
	err    error
}

func (p *CatalogPoller) Tick(ctx context.Context) (poller.Report, error) {
	catalogs, err := concurrency.FanOut(ctx, max(len(p.providers), 1), p.providers, fetch)
	if err != nil {
		return poller.Report{}, errs.Wrap(err, errs.CodeOf(err), "market.CatalogPoller.Tick")
	}
	var report poller.Report
	var failed []error
	for _, c := range catalogs {
		changed := 0
		if c.err == nil {
			changed, c.err = p.apply(ctx, c)
		}
		if c.err != nil {
			failed = append(failed, c.err)
			report.Attrs = append(report.Attrs, slog.String(string(c.issuer), string(errs.CodeOf(c.err))))
			continue
		}
		report.Scanned += len(c.assets)
		report.Changed += changed
		report.Attrs = append(report.Attrs, slog.Int(string(c.issuer), len(c.assets)))
	}
	checked, err := p.checkChainFacts(ctx)
	report.Changed += checked
	report.Attrs = append(report.Attrs, slog.Int("chain_checked", checked))
	return report, errors.Join(append(failed, err)...)
}

func fetch(ctx context.Context, provider AssetProvider) (issuerCatalog, error) {
	issuer := provider.Issuer()
	assets, err := provider.Catalog(ctx)
	if err == nil && len(assets) == 0 {
		err = errs.New(
			errs.CodeUpstreamUnavailable,
			"market.CatalogPoller.fetch",
			slog.String("issuer", string(issuer)),
			slog.String("reason", "empty catalog"),
		)
	}
	return issuerCatalog{issuer: issuer, assets: assets, err: err}, nil
}

func (p *CatalogPoller) apply(ctx context.Context, c issuerCatalog) (int, error) {
	now := p.clock.Now()
	rows := sqlc.UpsertAssetsParams{Issuer: string(c.issuer), Now: now}
	seen := map[domain.Mint]bool{}
	for _, a := range c.assets {
		if seen[a.Mint] {
			continue
		}
		seen[a.Mint] = true
		rows.Ids = append(rows.Ids, domain.NewAssetID(p.ids).UUID())
		rows.Symbols = append(rows.Symbols, a.Symbol)
		rows.Mints = append(rows.Mints, a.Mint.String())
		rows.Decimals = append(rows.Decimals, int16(a.Decimals))
		rows.Kinds = append(rows.Kinds, string(a.Kind))
		rows.DisplayNames = append(rows.DisplayNames, a.DisplayName)
		rows.LogoUrls = append(rows.LogoUrls, a.LogoURL)
		rows.IssuerTradables = append(rows.IssuerTradables, a.Tradable)
		rows.PopularRanks = append(rows.PopularRanks, domain.PopularRank(a.Symbol))
		rows.CompanyKeys = append(rows.CompanyKeys, domain.CompanyKey(a.DisplayName))
	}
	var changed int64
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		q := sqlc.New(tx.Queries())
		upserted, err := q.UpsertAssets(ctx, rows)
		if err != nil {
			return err
		}
		delisted, err := q.DelistMissingAssets(ctx,
			sqlc.DelistMissingAssetsParams{Now: now, Issuer: rows.Issuer, Listed: rows.Mints})
		changed = upserted + delisted
		return err
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "market.CatalogPoller.apply", slog.String("issuer", rows.Issuer))
	}
	return int(changed), nil
}
