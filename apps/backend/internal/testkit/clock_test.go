package testkit_test

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

func epoch() time.Time { return time.Date(2026, 9, 27, 12, 0, 0, 0, time.UTC) }

func received(ch <-chan time.Time) (time.Time, bool) {
	select {
	case v := <-ch:
		return v, true
	default:
		return time.Time{}, false
	}
}

func TestClockNowMovesOnlyWhenTold(t *testing.T) {
	t.Parallel()
	start := epoch()
	c := testkit.NewClock(start)
	if got := c.Now(); !got.Equal(start) {
		t.Fatalf("Now() = %v, want %v", got, start)
	}
	c.Advance(90 * time.Second)
	if got := c.Now(); !got.Equal(start.Add(90 * time.Second)) {
		t.Fatalf("after Advance(90s) Now() = %v", got)
	}
	later := start.Add(24 * time.Hour)
	c.Set(later)
	if got := c.Now(); !got.Equal(later) {
		t.Fatalf("after Set Now() = %v, want %v", got, later)
	}
}

func TestClockAfterFiresAtItsDeadline(t *testing.T) {
	t.Parallel()
	start := epoch()
	var c clock.Clock = testkit.NewClock(start)
	fake := c.(*testkit.Clock)
	ch := c.After(5 * time.Second)
	fake.Advance(4 * time.Second)
	if v, ok := received(ch); ok {
		t.Fatalf("After(5s) fired at %v after 4s", v)
	}
	fake.Advance(time.Second)
	v, ok := received(ch)
	if !ok || !v.Equal(start.Add(5*time.Second)) {
		t.Fatalf("After(5s) after 5s = %v, %v", v, ok)
	}
	fake.Advance(time.Hour)
	if v, ok := received(ch); ok {
		t.Fatalf("After fired twice, second at %v", v)
	}
}

func TestClockAfterNonPositiveFiresNow(t *testing.T) {
	t.Parallel()
	start := epoch()
	c := testkit.NewClock(start)
	v, ok := received(c.After(0))
	if !ok || !v.Equal(start) {
		t.Fatalf("After(0) = %v, %v; want %v at once", v, ok, start)
	}
}

func TestClockSetFiresTimersItPasses(t *testing.T) {
	t.Parallel()
	start := epoch()
	c := testkit.NewClock(start)
	ch := c.After(time.Minute)
	c.Set(start.Add(2 * time.Minute))
	v, ok := received(ch)
	if !ok || !v.Equal(start.Add(time.Minute)) {
		t.Fatalf("After(1m) after Set(+2m) = %v, %v", v, ok)
	}
}

func TestClockTickerTicksEachPeriodAndDropsWhenFull(t *testing.T) {
	t.Parallel()
	start := epoch()
	c := testkit.NewClock(start)
	ticker := c.NewTicker(time.Second)
	for want := 1; want <= 3; want++ {
		c.Advance(time.Second)
		v, ok := received(ticker.C())
		if !ok || !v.Equal(start.Add(time.Duration(want)*time.Second)) {
			t.Fatalf("tick %d = %v, %v", want, v, ok)
		}
	}
	c.Advance(10 * time.Second)
	v, ok := received(ticker.C())
	if !ok || !v.Equal(start.Add(4*time.Second)) {
		t.Fatalf("first buffered tick after a 10s jump = %v, %v; want the 4s tick", v, ok)
	}
	if v, ok := received(ticker.C()); ok {
		t.Fatalf("ticker buffered a second tick %v", v)
	}
	c.Advance(time.Second)
	v, ok = received(ticker.C())
	if !ok || !v.Equal(start.Add(14*time.Second)) {
		t.Fatalf("tick after the jump = %v, %v; want 14s", v, ok)
	}
}

func TestClockStoppedTickerNeverTicks(t *testing.T) {
	t.Parallel()
	start := epoch()
	c := testkit.NewClock(start)
	ticker := c.NewTicker(time.Second)
	ticker.Stop()
	c.Advance(time.Minute)
	if v, ok := received(ticker.C()); ok {
		t.Fatalf("stopped ticker ticked at %v", v)
	}
}

func TestClockTimersFireInDeadlineOrder(t *testing.T) {
	t.Parallel()
	start := epoch()
	c := testkit.NewClock(start)
	late := c.After(3 * time.Second)
	early := c.After(time.Second)
	c.Advance(5 * time.Second)
	e, _ := received(early)
	l, _ := received(late)
	if !e.Equal(start.Add(time.Second)) || !l.Equal(start.Add(3*time.Second)) {
		t.Fatalf("early fired at %v, late at %v", e, l)
	}
}

func TestClockTickerRejectsNonPositiveInterval(t *testing.T) {
	t.Parallel()
	start := epoch()
	defer func() {
		if recover() == nil {
			t.Fatal("NewTicker(0) did not panic")
		}
	}()
	testkit.NewClock(start).NewTicker(0)
}

func TestClockNotifyTickersReportsEachNewTickerInterval(t *testing.T) {
	t.Parallel()
	c := testkit.NewClock(epoch())
	made := make(chan time.Duration, 2)
	c.NotifyTickers(made)
	c.NewTicker(3 * time.Second)
	c.NewTicker(7 * time.Second)
	for _, want := range []time.Duration{3 * time.Second, 7 * time.Second} {
		if got := <-made; got != want {
			t.Fatalf("notified interval = %v, want %v", got, want)
		}
	}
}
