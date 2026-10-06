package domain

import "time"

const (
	RunEvery        = 2 * time.Minute
	TriggerDebounce = time.Second
)

func ShouldRun(now, lastRun time.Time, oldestTrigger *time.Time) bool {
	return now.Sub(lastRun) >= RunEvery || (oldestTrigger != nil && now.Sub(*oldestTrigger) >= TriggerDebounce)
}

func MaxSnapshotGap(r Range) time.Duration {
	switch r {
	case Range1W, Range1M, RangeAll:
		return time.Hour + 2*RunEvery
	case Range1H, Range1D:
	}
	return 3 * RunEvery
}
