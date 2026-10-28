package domain

import (
	"log/slog"
	"math"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Side = string

const (
	SideYes Side = "yes"
	SideNo  Side = "no"
)

const (
	CallBuy  = "buy"
	CallSell = "sell"
)

type PeriodKind string

const (
	PeriodWeek  PeriodKind = "week"
	PeriodMonth PeriodKind = "month"
	PeriodAll   PeriodKind = "all"
)

type Period struct {
	Kind  PeriodKind
	Start time.Time
	End   time.Time
}

func PeriodContaining(kind PeriodKind, t time.Time) Period {
	t = t.UTC()
	switch kind {
	case PeriodWeek:
		day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
		start := day.AddDate(0, 0, -((int(day.Weekday()) + 6) % 7))
		return Period{Kind: kind, Start: start, End: start.AddDate(0, 0, 7)}
	case PeriodMonth:
		start := time.Date(t.Year(), t.Month(), 1, 0, 0, 0, 0, time.UTC)
		return Period{Kind: kind, Start: start, End: start.AddDate(0, 1, 0)}
	case PeriodAll:
	}
	return Period{
		Kind:  PeriodAll,
		Start: time.Unix(0, 0).UTC(),
		End:   time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

func Right(kind, side Side, ref, end money.Micros) bool {
	cmp := end.Cmp(ref)
	switch {
	case kind == CallBuy && side == SideYes:
		return cmp > 0
	case kind == CallBuy && side == SideNo:
		return cmp <= 0
	case kind == CallSell && side == SideYes:
		return cmp < 0
	case kind == CallSell && side == SideNo:
		return cmp >= 0
	}
	return false
}

const bpsPerUnit = 10_000

func MoveBps(ref, end money.Micros) (int, error) {
	if ref.IsZero() {
		return 0, errs.New(errs.CodeInvalidInput, "ranking.MoveBps", slog.String("reason", "zero_reference"))
	}
	rises := end.Cmp(ref) > 0
	gap := ref.Uint64() - end.Uint64()
	if rises {
		gap = end.Uint64() - ref.Uint64()
	}
	bps, err := money.MulDiv(gap, bpsPerUnit, ref.Uint64())
	if err != nil || bps > math.MaxInt32 {
		return 0, errs.New(errs.CodeInvalidInput, "ranking.MoveBps",
			slog.Uint64("ref", ref.Uint64()), slog.Uint64("end", end.Uint64()))
	}
	if rises {
		return int(bps), nil
	}
	return -int(bps), nil
}

func FillPrice(e events.TradeConfirmed, decimals uint8) (money.Micros, error) {
	tokens := e.OutAmount
	if e.Action == CallSell {
		tokens = e.InAmount
	}
	price := money.PricePerUnit(e.USDCMicros, tokens, decimals)
	if tokens == 0 || price.IsZero() {
		return money.Micros{}, errs.New(errs.CodeInvalidInput, "ranking.FillPrice",
			slog.String("action", e.Action), slog.Uint64("tokens", tokens), slog.Int("decimals", int(decimals)))
	}
	return price, nil
}
