package domain

import "time"

const (
	RunEvery        = 2 * time.Minute
	TriggerDebounce = time.Second
)

func ShouldRun(now, lastRun time.Time, oldestTrigger *time.Time) bool {
	return now.Sub(lastRun) >= RunEvery || (oldestTrigger != nil && now.Sub(*oldestTrigger) >= TriggerDebounce)
}
