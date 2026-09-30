package domain

import (
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const StaleAfter = 5 * time.Minute

type Sample struct {
	Price money.Micros
	At    time.Time
}

type Session struct {
	Open       bool
	Continuous bool
	LastClose  time.Time
}

func PriceForBoard(s Session, latest, atClose *Sample, now time.Time) (Sample, []Flag) {
	sample, asOf := latest, now
	if !s.Open && !s.Continuous {
		sample, asOf = atClose, s.LastClose
	}
	if sample == nil {
		return Sample{}, []Flag{FlagUnpricedAssets}
	}
	if asOf.Sub(sample.At) > StaleAfter {
		return *sample, []Flag{FlagStalePrices}
	}
	return *sample, nil
}
