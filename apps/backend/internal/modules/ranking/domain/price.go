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
	Open            bool
	Continuous      bool
	LastClose       time.Time
	LastSessionOpen time.Time
}

func PriceForBoard(s Session, latest, atClose *Sample, now time.Time) (priced Sample, flags []Flag, closeMissing bool) {
	sample, asOf := latest, now
	offSession := !s.Open && !s.Continuous
	if offSession {
		sample, asOf = atClose, s.LastClose
	}
	if sample == nil {
		return Sample{}, []Flag{FlagUnpricedAssets}, false
	}
	if asOf.Sub(sample.At) <= StaleAfter {
		return *sample, nil, false
	}
	if offSession && !s.LastSessionOpen.IsZero() && !sample.At.Before(s.LastSessionOpen) && !sample.Price.IsZero() {
		return *sample, nil, true
	}
	return *sample, []Flag{FlagStalePrices}, false
}
