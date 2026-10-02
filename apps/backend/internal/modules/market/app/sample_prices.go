package app

import (
	"context"
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type priceReader interface {
	LatestPrices(context.Context) (map[domain.AssetID]domain.Sample, error)
	PricesAsOf(context.Context, []domain.AssetID, time.Time) (map[domain.AssetID]domain.Sample, error)
	DaySamples(context.Context, domain.AssetID, time.Time, time.Time) ([]domain.Sample, error)
}

type CorePublisher interface {
	PublishCore(ctx context.Context, m events.Core) error
}

type SamplePrices struct {
	uow      *db.UnitOfWork
	ids      ids.Generator
	clock    clock.Clock
	catalog  *Catalog
	book     priceReader
	source   PriceSource
	ticks    CorePublisher
	interval time.Duration
}

func NewSamplePrices(
	uow *db.UnitOfWork, reads sqlc.DBTX, g ids.Generator, c clock.Clock, source PriceSource,
	ticks CorePublisher, interval time.Duration,
) *SamplePrices {
	return &SamplePrices{
		uow: uow, ids: g, clock: c, catalog: NewCatalog(reads), book: NewPriceBook(reads, c), source: source,
		ticks: ticks, interval: interval,
	}
}

func (*SamplePrices) Name() string { return "market.prices" }

func (p *SamplePrices) Interval() time.Duration { return p.interval }

func (p *SamplePrices) Tick(ctx context.Context) (poller.Report, error) {
	assets, err := p.catalog.ListAll(ctx)
	if err != nil {
		return poller.Report{}, err
	}
	at := domain.Bucket(p.clock.Now())
	mints := make([]domain.Mint, len(assets))
	for i, a := range assets {
		mints[i] = a.Mint
	}
	answered, sampleErr := p.source.Prices(ctx, mints)
	rows := sqlc.InsertPricePointsParams{Ts: at, Source: string(domain.SourceJupiter)}
	for _, m := range mints {
		if stored, ok := storable(answered, m); ok {
			rows.Mints = append(rows.Mints, m.String())
			rows.PriceMicros = append(rows.PriceMicros, stored)
		}
	}
	written, err := p.insert(ctx, rows)
	var pubErr error
	if err == nil {
		pubErr = p.publish(ctx, at, assets)
	}
	if err == nil {
		err = p.recordMoves(ctx, assets, at)
	}
	if err == nil {
		err = pubErr
	}
	if err != nil {
		return poller.Report{}, err
	}
	missing := len(mints) - len(rows.Mints)
	report := poller.Report{Scanned: len(assets), Changed: written, Attrs: []slog.Attr{
		slog.Int("priced", len(rows.Mints)), slog.Int("missing", missing),
	}}
	if sampleErr != nil {
		return report, errs.Wrap(sampleErr, errs.CodeOf(sampleErr), "market.SamplePrices.Tick",
			slog.Int("written", written), slog.Int("missing", missing))
	}
	return report, nil
}

func storable(answered map[domain.Mint]money.Micros, m domain.Mint) (int64, bool) {
	micros, ok := answered[m]
	if !ok {
		return 0, false
	}
	signed, err := micros.Delta(money.Micros{})
	return signed.Int64(), err == nil
}

func (p *SamplePrices) insert(ctx context.Context, rows sqlc.InsertPricePointsParams) (int, error) {
	if len(rows.Mints) == 0 {
		return 0, nil
	}
	var written int64
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		written, err = sqlc.New(tx.Queries()).InsertPricePoints(ctx, rows)
		return err
	})
	if err != nil {
		return 0, errs.Wrap(err, errs.CodeOf(err), "market.SamplePrices.insert", slog.Int("rows", len(rows.Mints)))
	}
	return int(written), nil
}

func (p *SamplePrices) publish(ctx context.Context, at time.Time, assets []domain.Asset) error {
	accepted, err := p.book.LatestPrices(ctx)
	if err != nil {
		return err
	}
	tick := events.PriceTick{V: 1, AsOf: at, Prices: make([]events.TickPrice, 0, len(accepted))}
	for _, a := range assets {
		if s, ok := accepted[a.ID]; ok {
			tick.Prices = append(tick.Prices, events.TickPrice{
				Mint: a.Mint.Address(), AssetID: a.ID.UUID(), PriceMicros: s.Micros, ObservedAt: s.ObservedAt,
			})
		}
	}
	return p.ticks.PublishCore(ctx, tick)
}
