package poller_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

func TestSpread_staysBelowThePeriodAndTreatsANonPositivePeriodAsZero(t *testing.T) {
	t.Parallel()
	for range 1000 {
		if got := poller.Spread(time.Hour); got < 0 || got >= time.Hour {
			t.Fatalf("Spread(1h) = %v, want [0, 1h)", got)
		}
	}
	for _, period := range []time.Duration{0, -time.Hour} {
		if got := poller.Spread(period); got != 0 {
			t.Fatalf("Spread(%v) = %v, want 0", period, got)
		}
	}
}
