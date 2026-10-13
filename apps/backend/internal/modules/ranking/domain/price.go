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

func PriceForBoard(latest *Sample, now time.Time) (priced Sample, flags []Flag) {
	if latest == nil {
		return Sample{}, []Flag{FlagUnpricedAssets}
	}
	if now.Sub(latest.At) <= StaleAfter {
		return *latest, nil
	}
	return *latest, []Flag{FlagStalePrices}
}
