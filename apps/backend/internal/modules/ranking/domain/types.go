package domain

import (
	"log/slog"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
)

type Bps int64

type Range string

const (
	Range1H  Range = "1H"
	Range1D  Range = "1D"
	Range1W  Range = "1W"
	Range1M  Range = "1M"
	RangeAll Range = "ALL"
)

func ParseRange(raw string) (Range, error) {
	r := Range(raw)
	if _, ok := r.span(); ok || r == RangeAll {
		return r, nil
	}
	return "", errs.New(errs.CodeInvalidInput, "ranking.ParseRange", slog.String("raw", raw))
}

func (r Range) Start(now time.Time) (time.Time, bool) {
	span, ok := r.span()
	if !ok {
		return time.Time{}, false
	}
	return now.Add(-span), true
}

func (r Range) span() (time.Duration, bool) {
	switch r {
	case Range1H:
		return time.Hour, true
	case Range1D:
		return 24 * time.Hour, true
	case Range1W:
		return 7 * 24 * time.Hour, true
	case Range1M:
		return 30 * 24 * time.Hour, true
	case RangeAll:
	}
	return 0, false
}

type Board string

const (
	BoardCabals       Board = "cabals"
	BoardPeople       Board = "people"
	BoardCabalMembers Board = "cabal_members"
)

type Flag string

const (
	FlagStalePrices    Flag = "stale_prices"
	FlagUnpricedAssets Flag = "unpriced_assets"
)

type Filter string

const (
	FilterAll     Filter = "all"
	FilterFriends Filter = "friends"
)

func ParseFilter(raw string) (Filter, error) {
	switch f := Filter(raw); f {
	case FilterAll, FilterFriends:
		return f, nil
	}
	return "", errs.New(errs.CodeInvalidInput, "ranking.ParseFilter", slog.String("raw", raw))
}
