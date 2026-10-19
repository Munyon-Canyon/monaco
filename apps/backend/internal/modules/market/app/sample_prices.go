package app

import (
	"context"
	"log/slog"
	"math"
	"slices"
	"strconv"
	"strings"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/ids"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type priceReader interface {
	PointsCount(context.Context) (int64, error)
	RecentSamples(context.Context) (map[domain.AssetID][]domain.Sample, error)
	SamplesOf(context.Context, []domain.AssetID) (map[domain.AssetID][]domain.Sample, error)
	PricesAsOf(context.Context, []domain.AssetID, time.Time) (map[domain.AssetID]domain.Sample, error)
	DaySamples(context.Context, domain.AssetID, time.Time, time.Time) ([]domain.Sample, error)
}

type volumeBook interface {
	Volume24h(ctx context.Context, mint string, now time.Time) (*money.Micros, error)
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
	volume   volumeBook
	source   PriceSource
	ticks    CorePublisher
	interval time.Duration
	hot      []HotMints
	listings *PriceBook
	evm      EVMSampling
	dropped  map[ListingKey]time.Time

	cached  []domain.Asset
	version CatalogVersion
	loaded  bool

	window   map[domain.AssetID][]domain.Sample
	lastTick time.Time
	seededAt time.Time
	points   int64
	wrote    int64
}

func NewSamplePrices(
	uow *db.UnitOfWork, reads sqlc.DBTX, g ids.Generator, c clock.Clock, source PriceSource,
	ticks CorePublisher, interval time.Duration, hot ...HotMints,
) *SamplePrices {
	book := NewPriceBook(reads, c)
	return &SamplePrices{
		uow: uow, ids: g, clock: c, catalog: NewCatalog(reads), book: book, volume: book, source: source,
		ticks: ticks, interval: interval, hot: hot, listings: book,
	}
}

func (*SamplePrices) Name() string { return "market.prices" }

func (p *SamplePrices) Interval() time.Duration { return p.interval }

func (p *SamplePrices) Tick(ctx context.Context) (poller.Report, error) {
	assets, err := p.catalogAssets(ctx)
	if err != nil {
		return poller.Report{}, err
	}
	if err = p.ensureWindow(ctx); err != nil {
		return poller.Report{}, err
	}
	at := domain.Bucket(p.clock.Now())
	start := p.clock.Now()
	need, err := p.needs(ctx, assets, p.capacity())
	if err != nil {
		return poller.Report{}, err
	}
	asked := need.mints()
	fetchCtx, cancelFetch := context.WithTimeout(ctx, p.fetchWindow())
	answered, sampleErr := p.source.Prices(fetchCtx, asked)
	cancelFetch()
	rows := sqlc.InsertPricePointsParams{Ts: at, Source: string(domain.SourceJupiter)}
	var left []domain.Asset
	for _, a := range need.assets {
		if stored, ok := storable(answered, a.Mint); ok {
			rows.Mints = append(rows.Mints, a.Mint.String())
			rows.PriceMicros = append(rows.PriceMicros, stored)
		} else {
			left = append(left, a)
		}
	}
	inserted, err := p.insert(ctx, rows)
	if err != nil {
		return poller.Report{}, err
	}
	evmAttrs, err := p.settle(ctx, at, assets, answered, rows.Mints, inserted)
	if err != nil {
		p.window = nil
		return poller.Report{}, err
	}
	p.points += p.wrote
	written := len(inserted)
	missing := len(left)
	logMissing(ctx, left)
	report := poller.Report{Scanned: len(asked), Changed: written, Attrs: []slog.Attr{
		slog.Int("priced", len(rows.Mints)), slog.Int("missing", missing),
		slog.Int("hot", need.hot), slog.Int("total", len(asked)), slog.Int("failed_batches", 0),
	}}
	report.Attrs = append(report.Attrs, evmAttrs...)
	if sampleErr != nil {
		return report, errs.Wrap(sampleErr, errs.CodeOf(sampleErr), "market.SamplePrices.Tick",
			slog.Int("written", written), slog.Int("missing", missing),
			slog.Int("hot", need.hot), slog.Int("total", len(asked)),
			slog.Int64("duration_ms", p.clock.Now().Sub(start).Milliseconds()))
	}
	return report, nil
}

func (p *SamplePrices) catalogAssets(ctx context.Context) ([]domain.Asset, error) {
	version, err := p.catalog.Version(ctx)
	if err != nil {
		return nil, err
	}
	if p.loaded && version == p.version {
		return p.cached, nil
	}
	assets, err := p.catalog.ListAll(ctx)
	if err != nil {
		return nil, err
	}
	p.cached, p.version, p.loaded = assets, version, true
	p.window = nil
	return assets, nil
}

const priceTail = 15 * time.Second

func (p *SamplePrices) fetchWindow() time.Duration { return max(p.interval-priceTail, p.interval/2) }

func (p *SamplePrices) capacity() int {
	if c, ok := p.source.(CappedSource); ok {
		return c.MintsWithin(p.fetchWindow())
	}
	return math.MaxInt
}

const missingMintsLogged = 10

func logMissing(ctx context.Context, left []domain.Asset) {
	if len(left) == 0 {
		return
	}
	byIssuer := map[domain.Issuer]int{}
	first := make([]string, 0, missingMintsLogged)
	for _, a := range left {
		byIssuer[a.Issuer]++
		if len(first) < missingMintsLogged {
			first = append(first, a.Mint.String())
		}
	}
	issuers := make([]string, 0, len(byIssuer))
	for issuer, n := range byIssuer {
		issuers = append(issuers, string(issuer)+"="+strconv.Itoa(n))
	}
	slices.Sort(issuers)
	observability.Info(ctx, observability.MarketPricesMissing, slog.Int("missing", len(left)),
		slog.String("by_issuer", strings.Join(issuers, ",")), slog.String("mints", strings.Join(first, ",")))
}

func storable(answered map[domain.Mint]money.Micros, m domain.Mint) (int64, bool) {
	micros, ok := answered[m]
	if !ok {
		return 0, false
	}
	return storableMicros(micros)
}

func storableMicros(micros money.Micros) (int64, bool) {
	signed, err := micros.Delta(money.Micros{})
	return signed.Int64(), err == nil
}

func (p *SamplePrices) insert(
	ctx context.Context,
	rows sqlc.InsertPricePointsParams,
) ([]sqlc.InsertPricePointsRow, error) {
	if len(rows.Mints) == 0 {
		return nil, nil
	}
	var inserted []sqlc.InsertPricePointsRow
	err := p.uow.Do(ctx, func(ctx context.Context, tx db.Tx) error {
		var err error
		q := sqlc.New(tx.Queries())
		if inserted, err = q.InsertPricePoints(ctx, rows); err == nil {
			_, err = q.MarkAssetsQuoted(ctx, sqlc.MarkAssetsQuotedParams{At: p.clock.Now(), Mints: rows.Mints})
		}
		return err
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.SamplePrices.insert", slog.Int("rows", len(rows.Mints)))
	}
	p.wrote += int64(len(inserted))
	return inserted, nil
}

func (p *SamplePrices) settle(
	ctx context.Context, at time.Time, assets []domain.Asset, answered map[domain.Mint]money.Micros,
	asked []string, inserted []sqlc.InsertPricePointsRow,
) ([]slog.Attr, error) {
	if err := p.absorb(ctx, assets, answered, asked, inserted); err != nil {
		return nil, err
	}
	attrs, err := p.sampleListings(ctx, at, answered)
	if err != nil {
		return nil, err
	}
	accepted := p.accepted()
	pubErr := p.publish(ctx, at, assets, accepted)
	if err := p.recordMoves(ctx, assets, at, accepted); err != nil {
		return nil, err
	}
	return attrs, pubErr
}

func (p *SamplePrices) publish(
	ctx context.Context, at time.Time, assets []domain.Asset, accepted map[domain.AssetID]domain.Sample,
) error {
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
