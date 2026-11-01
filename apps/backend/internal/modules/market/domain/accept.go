package domain

import (
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/money"
)

const HoldWindow = 3

const (
	PriceFreshWindow      = 15 * time.Minute
	ClosedReferenceWindow = 24 * time.Hour
)

type Sample struct {
	Micros     money.Micros
	ObservedAt time.Time
	Source     Source
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

func AcceptNewest(newestFirst []Sample) (Sample, bool) {
	window := newestFirst[:min(len(newestFirst), HoldWindow)]
	if len(window) == 0 {
		return Sample{}, false
	}
	if len(window) == 1 || withinAFifth(window[0].Micros, window[1].Micros) {
		return window[0], true
	}
	return Sample{}, false
}

func WithinAFifth(next, prev money.Micros) bool { return withinAFifth(next, prev) }

func withinAFifth(next, prev money.Micros) bool {
	n, p := next.Uint64(), prev.Uint64()
	return max(n, p)-min(n, p) <= p/5
}

func (s Sample) Stale(session SessionInfo, now time.Time) bool {
	if session.State == StateClosed && !session.LastClose.IsZero() {
		return s.ObservedAt.Before(session.LastClose.Add(-ClosedReferenceWindow))
	}
	return now.Sub(s.ObservedAt) > PriceFreshWindow
}

func WithinOfClose(samples []Sample, lastClose time.Time) []Sample {
	if lastClose.IsZero() {
		return samples
	}
	floor := lastClose.Add(-ClosedReferenceWindow)
	out := make([]Sample, 0, len(samples))
	for _, s := range samples {
		if !s.ObservedAt.Before(floor) {
			out = append(out, s)
		}
	}
	return out
}
