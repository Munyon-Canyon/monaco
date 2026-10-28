package app

import (
	"context"
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/domain"
	"github.com/monaco/monaco/apps/backend/internal/modules/market/sqlc"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
)

const (
	yearDays         = 365
	yearCoverageDays = 7
)

type yearRangeReader interface {
	YearRange(context.Context, sqlc.YearRangeParams) (sqlc.YearRangeRow, error)
	UpsertHistoryDayRanges(context.Context, sqlc.UpsertHistoryDayRangesParams) error
}

var _ yearRangeReader = (*sqlc.Queries)(nil)

type YearRanges struct {
	read yearRangeReader
	hist PriceHistory
}

func NewYearRanges(db sqlc.DBTX) *YearRanges {
	return &YearRanges{read: sqlc.New(db)}
}

func (y *YearRanges) WithHistory(h PriceHistory) *YearRanges {
	y.hist = h
	return y
}

func (y *YearRanges) YearRange(
	ctx context.Context, mint domain.Mint, now time.Time,
) (low, high *money.Micros, err error) {
	const op = "market.YearRanges.YearRange"
	cutoff := now.UTC().AddDate(0, 0, -yearDays)
	row, err := y.read.YearRange(ctx, yearRangeParams(mint, cutoff))
	if err != nil {
		return nil, nil, errs.Wrap(err, errs.CodeOf(err), op, slog.String("mint", mint.String()))
	}
	if !coversYear(row) && y.hist != nil && y.hist.Configured() {
		if !y.fillFromHistory(ctx, mint, cutoff) {
			return nil, nil, nil
		}
		row, err = y.read.YearRange(ctx, yearRangeParams(mint, cutoff))
		if err != nil {
			return nil, nil, errs.Wrap(err, errs.CodeOf(err), op, slog.String("mint", mint.String()))
		}
	}
	if !coversYear(row) {
		return nil, nil, nil
	}
	return microsPtr(row.LowMicros), microsPtr(row.HighMicros), nil
}

func yearRangeParams(mint domain.Mint, cutoff time.Time) sqlc.YearRangeParams {
	return sqlc.YearRangeParams{Mint: mint.String(), After: cutoff}
}

func coversYear(row sqlc.YearRangeRow) bool {
	return row.OldestGapDays > 0 && row.OldestGapDays <= yearCoverageDays && row.LowMicros > 0 && row.HighMicros > 0
}

func microsPtr(v int64) *money.Micros {
	m, _ := money.SignedMicrosFromInt64(v).Micros()
	return &m
}

func (y *YearRanges) fillFromHistory(ctx context.Context, mint domain.Mint, cutoff time.Time) bool {
	history, err := y.hist.MarketChart(ctx, mint, historyDays(domain.Chart1Y))
	if err != nil || len(history.Prices) == 0 {
		if errs.CodeOf(err) != errs.CodeNotFound {
			observability.Degraded(ctx, observability.MarketChartHistoryFailed,
				slog.String("mint", mint.String()),
				slog.String("code", string(errs.CodeOf(err))), slog.Any("err", err))
		}
		return false
	}
	arg := dayRangesOf(mint, history.Prices, cutoff)
	if len(arg.Days) == 0 {
		return false
	}
	if err := y.read.UpsertHistoryDayRanges(ctx, arg); err != nil {
		observability.Degraded(ctx, observability.MarketChartHistoryFailed,
			slog.String("mint", mint.String()), slog.String("code", string(errs.CodeOf(err))), slog.Any("err", err))
		return false
	}
	return true
}

func dayRangesOf(mint domain.Mint, prices []Sample, cutoff time.Time) sqlc.UpsertHistoryDayRangesParams {
	type span struct{ low, high int64 }
	spans := map[time.Time]span{}
	floor := cutoff.UTC().Truncate(24 * time.Hour)
	for _, s := range prices {
		micros, ok := domain.MicrosToInt64(s.Price)
		day := s.At.UTC().Truncate(24 * time.Hour)
		if !ok || micros <= 0 || !day.After(floor) {
			continue
		}
		cur, seen := spans[day]
		if !seen {
			cur = span{low: micros, high: micros}
		}
		spans[day] = span{low: min(cur.low, micros), high: max(cur.high, micros)}
	}
	days := make([]time.Time, 0, len(spans))
	for day := range spans {
		days = append(days, day)
	}
	slices.SortFunc(days, func(a, b time.Time) int { return a.Compare(b) })
	arg := sqlc.UpsertHistoryDayRangesParams{Mint: mint.String()}
	for _, day := range days {
		arg.Days = append(arg.Days, day)
		arg.HighMicros = append(arg.HighMicros, spans[day].high)
		arg.LowMicros = append(arg.LowMicros, spans[day].low)
	}
	return arg
}
