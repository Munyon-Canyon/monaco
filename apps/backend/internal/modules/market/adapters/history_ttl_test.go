package adapters

import (
	"testing"
	"time"
)

func TestHistoryTTL_aClosedMarketDayWindowStaysShortAndLongerRangesStayLong(t *testing.T) {
	t.Parallel()
	want := map[int]time.Duration{
		1: historyShortTTL, 2: historyShortTTL, 4: historyShortTTL, historyShortDays: historyShortTTL,
		7: historyLongTTL, 30: historyLongTTL, -1: historyLongTTL,
	}
	for days, ttl := range want {
		if got := historyTTL(days); got != ttl {
			t.Fatalf("historyTTL(%d) = %s, want %s", days, got, ttl)
		}
	}
}
