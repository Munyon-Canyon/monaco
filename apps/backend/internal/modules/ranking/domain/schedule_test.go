package domain_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/ranking/domain"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

func TestShouldRun(t *testing.T) {
	t.Parallel()
	now := clock.Real{}.Now().UTC()
	ago := func(d time.Duration) *time.Time {
		at := now.Add(-d)
		return &at
	}
	tests := map[string]struct {
		lastRun time.Duration
		trigger *time.Time
		want    bool
	}{
		"timer due":                  {lastRun: domain.RunEvery, want: true},
		"timer not due":              {lastRun: domain.RunEvery - time.Nanosecond},
		"trigger settled":            {lastRun: time.Second, trigger: ago(domain.TriggerDebounce), want: true},
		"trigger still debouncing":   {lastRun: time.Second, trigger: ago(domain.TriggerDebounce - time.Nanosecond)},
		"timer due while debouncing": {lastRun: domain.RunEvery, trigger: ago(0), want: true},
	}
	for name, tt := range tests {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			if got := domain.ShouldRun(now, now.Add(-tt.lastRun), tt.trigger); got != tt.want {
				t.Fatalf("ShouldRun = %v, want %v", got, tt.want)
			}
		})
	}
}
