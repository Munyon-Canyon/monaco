package domain

import (
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type ChartRange string

const (
	Chart1D  ChartRange = "1D"
	Chart1W  ChartRange = "1W"
	Chart1M  ChartRange = "1M"
	Chart3M  ChartRange = "3M"
	Chart1Y  ChartRange = "1Y"
	ChartAll ChartRange = "ALL"
)

func ParseChartRange(raw string) (ChartRange, error) {
	switch ChartRange(raw) {
	case Chart1D, Chart1W, Chart1M, Chart3M, Chart1Y, ChartAll:
		return ChartRange(raw), nil
	default:
		return "", errs.New(errs.CodeInvalidInput, "market.ParseChartRange", slog.String("range", raw))
	}
}

func (r ChartRange) Bucket() time.Duration {
	switch r {
	case Chart1D:
		return 5 * time.Minute
	case Chart1W, Chart1M, Chart3M:
		return time.Hour
	case Chart1Y, ChartAll:
		return 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func (r ChartRange) Window() (time.Duration, bool) {
	const day = 24 * time.Hour
	switch r {
	case Chart1D:
		return day, true
	case Chart1W:
		return 7 * day, true
	case Chart1M:
		return 30 * day, true
	case Chart3M:
		return 90 * day, true
	case Chart1Y:
		return 365 * day, true
	case ChartAll:
		return 0, false
	default:
		return 0, false
	}
}
