package domain

import (
	"log/slog"
	"math"
	"math/big"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

type Kind string

const (
	KindBuy  Kind = "buy"
	KindSell Kind = "sell"
)

func ParseKind(raw string) (Kind, bool) {
	switch k := Kind(raw); k {
	case KindBuy, KindSell:
		return k, true
	}
	return "", false
}

type Side string

const (
	SideYes Side = "yes"
	SideNo  Side = "no"
)

const bpsOfWhole = 10_000

func Right(kind Kind, side Side, ref, end money.Micros) bool {
	moved := end.Cmp(ref)
	switch kind {
	case KindBuy:
	case KindSell:
		moved = -moved
	default:
		return false
	}
	switch side {
	case SideYes:
		return moved > 0
	case SideNo:
		return moved <= 0
	}
	return false
}

func MoveBps(ref, end money.Micros) (int, error) {
	const op = "ranking.MoveBps"
	if ref.IsZero() {
		return 0, errs.New(errs.CodeInvalidInput, op)
	}
	delta, err := end.Delta(ref)
	if err != nil {
		return 0, err
	}
	moved := delta.Int64()
	magnitude := new(big.Int).Abs(big.NewInt(moved)).Uint64()
	bps, err := money.MulDiv(magnitude, bpsOfWhole, ref.Uint64())
	if err != nil {
		return 0, err
	}
	if bps > math.MaxInt32 {
		return 0, errs.New(errs.CodeInvalidInput, op, slog.Uint64("bps", bps))
	}
	if moved < 0 {
		return -int(bps), nil
	}
	return int(bps), nil
}

func FillPrice(e events.TradeConfirmed, decimals uint8) (money.Micros, error) {
	const op = "ranking.FillPrice"
	tokens := e.OutAmount
	if Kind(e.Action) == KindSell {
		tokens = e.InAmount
	}
	price := money.PricePerUnit(e.USDCMicros, tokens, decimals)
	if price.IsZero() {
		return money.Micros{}, errs.New(errs.CodeInvalidInput, op,
			slog.Uint64("tokens", tokens), slog.Int("decimals", int(decimals)))
	}
	return price, nil
}

type PeriodKind string

const (
	PeriodWeek  PeriodKind = "week"
	PeriodMonth PeriodKind = "month"
	PeriodAll   PeriodKind = "all"
)

type Period struct {
	Kind       PeriodKind
	Start, End time.Time
}

func PeriodContaining(kind PeriodKind, t time.Time) (Period, error) {
	t = t.UTC()
	day := time.Date(t.Year(), t.Month(), t.Day(), 0, 0, 0, 0, time.UTC)
	switch kind {
	case PeriodWeek:
		sinceMonday := (int(day.Weekday()) + 6) % 7
		start := day.AddDate(0, 0, -sinceMonday)
		return Period{Kind: kind, Start: start, End: start.AddDate(0, 0, 7)}, nil
	case PeriodMonth:
		start := day.AddDate(0, 0, 1-day.Day())
		return Period{Kind: kind, Start: start, End: start.AddDate(0, 1, 0)}, nil
	case PeriodAll:
		return Period{Kind: kind, Start: time.Unix(0, 0).UTC(), End: time.Date(9999, 1, 1, 0, 0, 0, 0, time.UTC)}, nil
	}
	return Period{}, errs.New(errs.CodeInvalidInput, "ranking.PeriodContaining", slog.String("kind", string(kind)))
}
