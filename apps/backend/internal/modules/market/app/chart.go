package app

import (
	"context"
	"database/sql"
	"errors"
	"log/slog"
	"math"
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

const (
	defaultChartLimit = 500
	maxChartLimit     = 1000
	chartOriginUnix   = 946684800
	chartMondayOrigin = chartOriginUnix + 2*24*60*60
)

type ChartRequest struct {
	Range, Interval string
	Before          *time.Time
	Limit           int
}

type Chart struct {
	Range         domain.ChartRange
	Interval      domain.ChartInterval
	Bucket        time.Duration
	Points        []Point
	Empty         bool
	NextBefore    *time.Time
	PreviousClose *money.Micros
}

type Charter interface {
	Handle(context.Context, string, ChartRequest) (Chart, error)
}

type chartReader interface {
	chartHistory
	AssetBySymbol(context.Context, string) (sqlc.Asset, error)
	NewestSamples(context.Context, sqlc.NewestSamplesParams) ([]sqlc.NewestSamplesRow, error)
	EarliestPrice(context.Context, string) (time.Time, error)
	InsertPendingBackfills(context.Context, sqlc.InsertPendingBackfillsParams) (int64, error)
}

type chartHistory interface {
	ChartBucketsPage(context.Context, sqlc.ChartBucketsPageParams) ([]sqlc.ChartBucketsPageRow, error)
	HasSampleBefore(context.Context, sqlc.HasSampleBeforeParams) (bool, error)
}

var _ Charter = (*AssetChart)(nil)

var _ dayBaseReader = (*sqlc.Queries)(nil)

var _ chartReader = (*sqlc.Queries)(nil)

type AssetChart struct {
	read  chartReader
	base  dayBaseReader
	clock clock.Clock
}

func NewChart(db sqlc.DBTX, c clock.Clock) *AssetChart {
	queries := sqlc.New(db)
	return &AssetChart{read: queries, base: queries, clock: c}
}

func (c *AssetChart) Handle(ctx context.Context, symbol string, req ChartRequest) (Chart, error) {
	span, interval, limit, err := parseChartRequest(req)
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
	c.queueBackfill(ctx, mint, now)
	until, priced, err := c.until(ctx, mint, now)
	if err != nil {
		return Chart{}, err
	}
	out := Chart{Range: span, Interval: interval, Bucket: interval.Duration(), Points: []Point{}}
	if !priced {
		out.Empty = true
		return out, nil
	}
	since, until, ok, err := c.readWindow(ctx, asset, req, span, out.Bucket, until, now)
	if err != nil || !ok {
		out.Empty = err == nil
		return out, err
	}
	if out.Points, err = c.points(ctx, mint, out.Bucket, since, until, limit); err != nil {
		return Chart{}, err
	}
	out.Empty = len(out.Points) == 0
	oldest, older, err := c.nextBefore(ctx, mint, out.Points)
	if err != nil {
		return Chart{}, err
	}
	if older {
		out.NextBefore = &oldest
	}
	if span == domain.Chart1D && req.Before == nil {
		out.PreviousClose, err = c.previousCloseOrNil(ctx, asset, now)
	}
	return out, err
}

func (c *AssetChart) readWindow(
	ctx context.Context,
	asset domain.Asset,
	req ChartRequest,
	span domain.ChartRange,
	width time.Duration,
	until, now time.Time,
) (time.Time, time.Time, bool, error) {
	if req.Before != nil {
		until = earlier(until, alignDown(*req.Before, width).Add(-time.Microsecond))
	}
	since, ok, err := c.since(ctx, asset, span, req.Before != nil, now)
	return since, until, ok, err
}

func (c *AssetChart) queueBackfill(ctx context.Context, mint string, now time.Time) {
	if _, err := c.read.InsertPendingBackfills(ctx,
		sqlc.InsertPendingBackfillsParams{Mints: []string{mint}, Now: now}); err != nil {
		observability.Degraded(ctx, observability.MarketChartBackfillQueueFailed, slog.String("mint", mint),
			slog.String("code", string(errs.CodeOf(err))), slog.Any("err", err))
	}
}

func (c *AssetChart) previousCloseOrNil(ctx context.Context, asset domain.Asset, now time.Time) (*money.Micros, error) {
	base, known, err := c.previousClose(ctx, asset, now)
	if err != nil || !known {
		return nil, err
	}
	return &base, nil
}

func parseChartRequest(req ChartRequest) (domain.ChartRange, domain.ChartInterval, int, error) {
	span, err := domain.ParseChartRange(req.Range)
	if err != nil {
		return "", "", 0, err
	}
	interval := span.DefaultInterval()
	if req.Interval != "" {
		if interval, err = domain.ParseChartInterval(req.Interval); err != nil {
			return "", "", 0, err
		}
	}
	if !span.Allows(interval) {
		return "", "", 0, errs.New(errs.CodeInvalidInput, "market.ChartRequest",
			slog.String("range", req.Range), slog.String("interval", string(interval)))
	}
	if req.Limit < 0 || req.Limit > maxChartLimit {
		return "", "", 0, errs.New(errs.CodeInvalidInput, "market.ChartRequest", slog.Int("limit", req.Limit))
	}
	limit := req.Limit
	switch {
	case limit > 0:
	case req.Before == nil:
		limit = math.MaxInt32
	default:
		limit = defaultChartLimit
	}
	return span, interval, limit, nil
}

func alignDown(at time.Time, width time.Duration) time.Time {
	w := int64(width / time.Second)
	origin := int64(chartOriginUnix)
	if width == domain.Interval1w.Duration() {
		origin = chartMondayOrigin
	}
	secs := at.Unix() - origin
	return time.Unix(origin+secs-((secs%w)+w)%w, 0).UTC()
}

func earlier(a, b time.Time) time.Time {
	if b.Before(a) {
		return b
	}
	return a
}

func (c *AssetChart) nextBefore(ctx context.Context, mint string, points []Point) (time.Time, bool, error) {
	if len(points) == 0 {
		return time.Time{}, false, nil
	}
	oldest := points[0].At
	older, err := c.read.HasSampleBefore(ctx, sqlc.HasSampleBeforeParams{Mint: mint, Before: oldest})
	if err != nil {
		return time.Time{}, false, errs.Wrap(err, errs.CodeOf(err), "market.HasSampleBefore", slog.String("mint", mint))
	}
	return oldest, older, nil
}

func (c *AssetChart) previousClose(
	ctx context.Context, asset domain.Asset, now time.Time,
) (money.Micros, bool, error) {
	book, err := c.dayBase(ctx, asset, now)
	if err != nil {
		return money.Micros{}, false, err
	}
	base, known := reference(asset, book)
	return base, known, nil
}

func (c *AssetChart) dayBase(ctx context.Context, asset domain.Asset, now time.Time) (quotes, error) {
	mints := []string{asset.Mint.String()}
	if asset.Kind == domain.KindPreIPO {
		opened, since, err := openedSamples(ctx, c.base, mints, now)
		return quotes{opened: opened, openedSince: since}, err
	}
	session, err := domain.Session(asset.Kind, now)
	if err != nil {
		return quotes{}, err
	}
	closed, err := closedSamples(ctx, c.base, mints, session)
	return quotes{closed: closed}, err
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
	ctx context.Context, asset domain.Asset, span domain.ChartRange, paging bool, now time.Time,
) (time.Time, bool, error) {
	mint := asset.Mint.String()
	if window, bounded := span.Window(); bounded && !paging {
		if opened, ok := lastSessionOpen(asset, span, now); ok {
			return opened, true, nil
		}
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

func lastSessionOpen(asset domain.Asset, span domain.ChartRange, now time.Time) (time.Time, bool) {
	if span != domain.Chart1D || asset.Kind != domain.KindEquity {
		return time.Time{}, false
	}
	session, err := domain.Session(asset.Kind, now)
	if err != nil || session.State == domain.StateOpen || session.LastSessionOpen.IsZero() {
		return time.Time{}, false
	}
	return session.LastSessionOpen, true
}

func (c *AssetChart) points(
	ctx context.Context, mint string, width time.Duration, since, until time.Time, limit int,
) ([]Point, error) {
	rows, err := c.read.ChartBucketsPage(ctx, sqlc.ChartBucketsPageParams{
		BucketSeconds: int64(
			width / time.Second,
		),
		Mint:      mint,
		Since:     since,
		Until:     until,
		PageLimit: int32(max(0, min(limit, math.MaxInt32))),
	})
	if err != nil {
		return nil, errs.Wrap(err, errs.CodeOf(err), "market.ChartBucketsPage", slog.String("mint", mint))
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

func pointOf(row sqlc.ChartBucketsPageRow) (Point, error) {
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
