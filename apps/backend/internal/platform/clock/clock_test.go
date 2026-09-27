package clock_test

import (
	"testing"
	"testing/synctest"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

func TestRealReadsTheBubbleClock(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var c clock.Clock = clock.Real{}
		start := c.Now()
		<-c.After(5 * time.Second)
		if got := c.Now().Sub(start); got != 5*time.Second {
			t.Fatalf("After(5s) fired after %v", got)
		}
	})
}

func TestRealTickerTicksUntilStopped(t *testing.T) {
	t.Parallel()
	synctest.Test(t, func(t *testing.T) {
		var c clock.Clock = clock.Real{}
		start := c.Now()
		ticker := c.NewTicker(time.Second)
		for want := 1; want <= 3; want++ {
			tick := <-ticker.C()
			if got := tick.Sub(start); got != time.Duration(want)*time.Second {
				t.Fatalf("tick %d at %v, want %ds", want, got, want)
			}
		}
		ticker.Stop()
		<-c.After(10 * time.Second)
		select {
		case tick := <-ticker.C():
			t.Fatalf("ticker ticked at %v after Stop", tick.Sub(start))
		default:
		}
	})
}
