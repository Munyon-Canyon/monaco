package domain

import (
	"log/slog"
	"slices"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type ChartRange string

const (
	Chart1H  ChartRange = "1H"
	Chart1D  ChartRange = "1D"
	Chart1W  ChartRange = "1W"
	Chart1M  ChartRange = "1M"
	Chart3M  ChartRange = "3M"
	Chart6M  ChartRange = "6M"
	Chart1Y  ChartRange = "1Y"
	ChartAll ChartRange = "ALL"
)

func ParseChartRange(raw string) (ChartRange, error) {
	switch ChartRange(raw) {
	case Chart1H, Chart1D, Chart1W, Chart1M, Chart3M, Chart6M, Chart1Y, ChartAll:
		return ChartRange(raw), nil
	default:
		return "", errs.New(errs.CodeInvalidInput, "market.ParseChartRange", slog.String("range", raw))
	}
}

type ChartInterval string

const (
	Interval1m  ChartInterval = "1m"
	Interval5m  ChartInterval = "5m"
	Interval15m ChartInterval = "15m"
	Interval30m ChartInterval = "30m"
	Interval1h  ChartInterval = "1h"
	Interval4h  ChartInterval = "4h"
	Interval1d  ChartInterval = "1d"
	Interval1w  ChartInterval = "1w"
)

func ParseChartInterval(raw string) (ChartInterval, error) {
	switch ChartInterval(raw) {
	case Interval1m, Interval5m, Interval15m, Interval30m, Interval1h, Interval4h, Interval1d, Interval1w:
		return ChartInterval(raw), nil
	default:
		return "", errs.New(errs.CodeInvalidInput, "market.ParseChartInterval", slog.String("interval", raw))
	}
}

func (i ChartInterval) Duration() time.Duration {
	switch i {
	case Interval1m:
		return time.Minute
	case Interval5m:
		return 5 * time.Minute
	case Interval15m:
		return 15 * time.Minute
	case Interval30m:
		return 30 * time.Minute
	case Interval1h:
		return time.Hour
	case Interval4h:
		return 4 * time.Hour
	case Interval1d:
		return 24 * time.Hour
	case Interval1w:
		return 7 * 24 * time.Hour
	default:
		return 24 * time.Hour
	}
}

func (r ChartRange) DefaultInterval() ChartInterval {
	switch r {
	case Chart1H:
		return Interval1m
	case Chart1D:
		return Interval5m
	case Chart1W, Chart1M, Chart3M:
		return Interval1h
	case Chart6M, Chart1Y, ChartAll:
		return Interval1d
	default:
		return Interval1d
	}
}

func (r ChartRange) Bucket() time.Duration { return r.DefaultInterval().Duration() }

func (r ChartRange) allowed() []ChartInterval {
	switch r {
	case Chart1H:
		return []ChartInterval{Interval1m, Interval5m}
	case Chart1D, Chart1W:
		return []ChartInterval{Interval5m, Interval15m, Interval30m, Interval1h, Interval4h, Interval1d}
	case Chart1M:
		return []ChartInterval{Interval15m, Interval30m, Interval1h, Interval4h, Interval1d}
	case Chart3M:
		return []ChartInterval{Interval30m, Interval1h, Interval4h, Interval1d, Interval1w}
	case Chart6M:
		return []ChartInterval{Interval1h, Interval4h, Interval1d, Interval1w}
	case Chart1Y, ChartAll:
		return []ChartInterval{Interval4h, Interval1d, Interval1w}
	default:
		return nil
	}
}

func (r ChartRange) Allows(i ChartInterval) bool { return slices.Contains(r.allowed(), i) }

func (r ChartRange) Window() (time.Duration, bool) {
	const day = 24 * time.Hour
	switch r {
	case Chart1H:
		return time.Hour, true
	case Chart1D:
		return day, true
	case Chart1W:
		return 7 * day, true
	case Chart1M:
		return 30 * day, true
	case Chart3M:
		return 90 * day, true
	case Chart6M:
		return 180 * day, true
	case Chart1Y:
		return 365 * day, true
	case ChartAll:
		return 0, false
	default:
		return 0, false
	}
}
