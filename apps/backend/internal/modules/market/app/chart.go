package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

type Point struct {
	At                     time.Time
	Open, High, Low, Close money.Micros
}

type Chart struct {
	Range  domain.ChartRange
	Bucket time.Duration
	Points []Point
	Empty  bool
}

type Charter interface {
	Handle(context.Context, string, string) (Chart, error)
}

type chartReader interface {
	AssetBySymbol(context.Context, string) (sqlc.Asset, error)
	NewestSamples(context.Context, sqlc.NewestSamplesParams) ([]sqlc.NewestSamplesRow, error)
	EarliestPrice(context.Context, string) (time.Time, error)
	ChartBuckets(context.Context, sqlc.ChartBucketsParams) ([]sqlc.ChartBucketsRow, error)
	InsertPendingBackfills(context.Context, sqlc.InsertPendingBackfillsParams) (int64, error)
}

var _ Charter = (*AssetChart)(nil)

var _ chartReader = (*sqlc.Queries)(nil)

type AssetChart struct {
	read  chartReader
	clock clock.Clock
}

func NewChart(db sqlc.DBTX, c clock.Clock) *AssetChart {
	return &AssetChart{read: sqlc.New(db), clock: c}
}

func (c *AssetChart) Handle(ctx context.Context, symbol, raw string) (Chart, error) {
	span, err := domain.ParseChartRange(raw)
	if err != nil {
		return Chart{}, err
	}
	row, err := c.read.AssetBySymbol(ctx, symbol)
	asset, err := one(row, err, "market.AssetBySymbol", slog.String("symbol", symbol))
	if err != nil {
		return Chart{}, err
	}
	now := c.clock.Now()
	mint := asset.Mint.String()
	if _, err := c.read.InsertPendingBackfills(ctx,
		sqlc.InsertPendingBackfillsParams{Mints: []string{mint}, Now: now}); err != nil {
		observability.Degraded(ctx, observability.MarketChartBackfillQueueFailed, slog.String("mint", mint),
			slog.String("code", string(errs.CodeOf(err))), slog.Any("err", err))
	}
	until, priced, err := c.until(ctx, mint, now)
	if err != nil {
		return Chart{}, err
	}
	out := Chart{Range: span, Bucket: span.Bucket(), Points: []Point{}}
	if !priced {
		out.Empty = true
		return out, nil
	}
	since, ok, err := c.since(ctx, mint, span, now)
	if err != nil {
		return Chart{}, err
	}
	if !ok {
		out.Empty = true
		return out, nil
	}
	points, err := c.points(ctx, mint, span, since, until)
	if err != nil {
		return Chart{}, err
	}
	out.Points = points
	out.Empty = len(points) == 0
	return out, nil
}

func (c *AssetChart) until(ctx context.Context, mint string, now time.Time) (time.Time, bool, error) {
	rows, err := c.read.NewestSamples(ctx, sqlc.NewestSamplesParams{Mints: []string{mint}, At: now})
	if err != nil {
		return time.Time{}, false, errs.Wrap(err, errs.CodeOf(err), "market.NewestSamples", slog.String("mint", mint))
	}
	samples, err := newestFirst(rows)
	if err != nil {
		return time.Time{}, false, err
	}
	_, accepted, ok := domain.DisplayUntil(now, samples)
	if !ok {
		return time.Time{}, false, nil
	}
	return accepted.ObservedAt, true, nil
}

func (c *AssetChart) since(
	ctx context.Context, mint string, span domain.ChartRange, now time.Time,
) (time.Time, bool, error) {
	if window, bounded := span.Window(); bounded {
		return now.Add(-window), true, nil
	}
	ts, err := c.read.EarliestPrice(ctx, mint)
	if errors.Is(err, sql.ErrNoRows) {
		return time.Time{}, false, nil
	}
	if err != nil {
		return time.Time{}, false, errs.Wrap(err, errs.CodeOf(err), "market.EarliestPrice", slog.String("mint", mint))
	}
	return ts, true, nil
}

func (c *AssetChart) points(
	ctx context.Context, mint string, span domain.ChartRange, since, until time.Time,
) ([]Point, error) {
	rows, err := c.read.ChartBuckets(ctx, sqlc.ChartBucketsParams{
		BucketSeconds: int64(span.Bucket() / time.Second), Mint: mint, Since: since, Until: until,
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.ChartBuckets", slog.String("mint", mint))
	}
	out := make([]Point, 0, len(rows))
	for _, row := range rows {
		point, pointErr := pointOf(row)
		if pointErr != nil {
			return nil, pointErr
		}
		out = append(out, point)
	}
	return out, nil
}

func newestFirst(rows []sqlc.NewestSamplesRow) ([]domain.Sample, error) {
	out := make([]domain.Sample, 0, len(rows))
	for _, row := range rows {
		sample, err := priceSample(row.Ts, row.PriceMicros)
		if err != nil {
			return nil, err
		}
		out = append(out, sample)
	}
	slices.SortFunc(out, func(a, b domain.Sample) int { return b.ObservedAt.Compare(a.ObservedAt) })
	return out, nil
}

func pointOf(row sqlc.ChartBucketsRow) (Point, error) {
	open, err := priceSample(row.Bucket, row.OpenMicros)
	high, highErr := priceSample(row.Bucket, row.HighMicros)
	low, lowErr := priceSample(row.Bucket, row.LowMicros)
	last, lastErr := priceSample(row.Bucket, row.CloseMicros)
	if err = errors.Join(err, highErr, lowErr, lastErr); err != nil {
		return Point{}, err
	}
	return Point{
		At: row.Bucket, Open: open.Micros, High: high.Micros, Low: low.Micros, Close: last.Micros,
	}, nil
}
