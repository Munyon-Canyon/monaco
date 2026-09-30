package domain

import (
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const HoldWindow = 3

type Sample struct {
	Micros     money.Micros
	ObservedAt time.Time
}

func Accept(newestFirst []Sample) (Sample, bool) {
	window := newestFirst[:min(len(newestFirst), HoldWindow)]
	for i, s := range window {
		if i == len(window)-1 || withinAFifth(s.Micros, window[i+1].Micros) {
			return s, true
		}
	}
	return Sample{}, false
}

func withinAFifth(next, prev money.Micros) bool {
	n, p := next.Uint64(), prev.Uint64()
	return max(n, p)-min(n, p) <= p/5
}
