package app

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	hoursPerDay = 24
	coldSlack   = 2 * time.Hour
)

func historyDays(span domain.ChartRange) int {
	window, bounded := span.Window()
	if !bounded {
		return HistoryMaxDays
	}
	return int((window + hoursPerDay*time.Hour - 1) / (hoursPerDay * time.Hour))
}

func (c *AssetChart) fromHistory(
	ctx context.Context,
	asset domain.Asset,
	req ChartRequest,
	span domain.ChartRange,
	interval domain.ChartInterval,
	limit int,
	now time.Time,
) (Chart, bool, error) {
	if c.hist == nil || !c.hist.Configured() {
		return Chart{}, false, nil
	}
	if out, served, err := c.fromColdStart(ctx, asset, req, span, interval, limit, now); served || err != nil {
		return out, served, err
	}
	history, err := c.hist.MarketChart(ctx, asset.Mint, historyDays(span))
	if err != nil || len(history.Prices) == 0 {
		if errs.CodeOf(err) != errs.CodeNotFound {
			observability.Degraded(ctx, observability.MarketChartHistoryFailed,
				slog.String("mint", asset.Mint.String()),
				slog.String("code", string(errs.CodeOf(err))), slog.Any("err", err))
		}
		return Chart{}, false, nil
	}
	width := interval.Duration()
	since, until := chartWindow(asset, req, span, interval, now)
	points, older := bucketHistory(history.Prices, width, since, until, limit)
	out := Chart{Range: span, Interval: interval, Bucket: width, Points: points, Empty: len(points) == 0}
	if older {
		oldest := points[0].At
		out.NextBefore = &oldest
	}
	if span == domain.Chart1D && req.Before == nil {
		var prevErr error
		out.PreviousClose, prevErr = c.previousCloseOrNil(ctx, asset, now)
		return out, true, prevErr
	}
	return out, true, nil
}

func (c *AssetChart) fromColdStart(
	ctx context.Context,
	asset domain.Asset,
	req ChartRequest,
	span domain.ChartRange,
	interval domain.ChartInterval,
	limit int,
	now time.Time,
) (Chart, bool, error) {
	peeker, ok := c.hist.(HistoryPeeker)
	if !ok || req.Before != nil {
		return Chart{}, false, nil
	}
	if _, found, _ := peeker.Peek(ctx, asset.Mint, historyDays(span)); found {
		return Chart{}, false, nil
	}
	out, err := c.fromPricePoints(ctx, asset, req, span, interval, limit, now)
	if err != nil {
		return Chart{}, false, err
	}
	return out, coversWindow(out, asset, req, span, now), nil
}

func coversWindow(chart Chart, asset domain.Asset, req ChartRequest, span domain.ChartRange, now time.Time) bool {
	if len(chart.Points) == 0 {
		return false
	}
	since, _ := chartWindow(asset, req, span, chart.Interval, now)
	return !since.IsZero() && chart.Points[0].At.Sub(since) <= max(coldSlack, 2*chart.Bucket)
}

func chartWindow(
	asset domain.Asset, req ChartRequest, span domain.ChartRange, interval domain.ChartInterval, now time.Time,
) (time.Time, time.Time) {
	width := interval.Duration()
	until := now
	if req.Before != nil {
		until = earlier(until, alignDown(*req.Before, width).Add(-time.Microsecond))
	}
	window, bounded := span.Window()
	if !bounded || req.Before != nil {
		return time.Time{}, until
	}
	if opened, ok := lastSessionOpen(asset, span, now); ok {
		return opened, until
	}
	return windowStart(span, now, window, interval), until
}

func bucketHistory(samples []Sample, width time.Duration, since, until time.Time, limit int) ([]Point, bool) {
	upto := make([]Sample, 0, len(samples))
	for _, s := range samples {
		if !s.At.After(until) {
			upto = append(upto, s)
		}
	}
	slices.SortStableFunc(upto, func(a, b Sample) int { return a.At.Compare(b.At) })
	points := make([]Point, 0, len(upto))
	for _, s := range upto {
		if !s.At.Before(since) {
			points = foldSample(points, s, alignDown(s.At, width))
		}
	}
	if len(points) > limit {
		points = points[len(points)-limit:]
	}
	if len(points) == 0 {
		return points, false
	}
	return points, upto[0].At.Before(points[0].At)
}

func foldSample(points []Point, s Sample, at time.Time) []Point {
	n := len(points)
	if n == 0 || !points[n-1].At.Equal(at) {
		return append(points, Point{At: at, Open: s.Price, High: s.Price, Low: s.Price, Close: s.Price})
	}
	last := &points[n-1]
	last.Close = s.Price
	if s.Price.Cmp(last.High) > 0 {
		last.High = s.Price
	}
	if s.Price.Cmp(last.Low) < 0 {
		last.Low = s.Price
	}
	return points
}
