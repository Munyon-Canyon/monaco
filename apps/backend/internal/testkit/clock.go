package testkit

import (
	"slices"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

type Clock struct {
	mu      sync.Mutex
	now     time.Time
	pending []*timer
	tickers chan<- time.Duration
}

type timer struct {
	clock  *Clock
	at     time.Time
	period time.Duration
	ch     chan time.Time
}

var _ clock.Clock = (*Clock)(nil)

func NewClock(start time.Time) *Clock { return &Clock{now: start} }

func (c *Clock) Now() time.Time {
	c.mu.Lock()
	defer c.mu.Unlock()
	return c.now
}

func (c *Clock) After(d time.Duration) <-chan time.Time {
	return c.schedule(d, 0).ch
}

func (c *Clock) NewTicker(d time.Duration) clock.Ticker {
	if d <= 0 {
		panic("testkit.Clock.NewTicker: non-positive interval")
	}
	c.mu.Lock()
	notify := c.tickers
	c.mu.Unlock()
	if notify != nil {
		select {
		case notify <- d:
		default:
		}
	}
	return c.schedule(d, d)
}

func (c *Clock) NotifyTickers(ch chan<- time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.tickers = ch
}

func (c *Clock) Advance(d time.Duration) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.moveTo(c.now.Add(d))
}

func (c *Clock) Set(t time.Time) {
	c.mu.Lock()
	defer c.mu.Unlock()
	c.moveTo(t)
}

func (c *Clock) schedule(d, period time.Duration) *timer {
	c.mu.Lock()
	defer c.mu.Unlock()
	t := &timer{clock: c, at: c.now.Add(d), period: period, ch: make(chan time.Time, 1)}
	c.pending = append(c.pending, t)
	c.moveTo(c.now)
	return t
}

func (c *Clock) moveTo(target time.Time) {
	for {
		i := c.nextDue(target)
		if i < 0 {
			break
		}
		t := c.pending[i]
		c.now = t.at
		select {
		case t.ch <- t.at:
		default:
		}
		if t.period > 0 {
			t.at = t.at.Add(t.period)
		} else {
			c.pending = slices.Delete(c.pending, i, i+1)
		}
	}
	c.now = target
}

func (c *Clock) nextDue(target time.Time) int {
	due := -1
	for i, t := range c.pending {
		if !t.at.After(target) && (due < 0 || t.at.Before(c.pending[due].at)) {
			due = i
		}
	}
	return due
}

func (t *timer) C() <-chan time.Time { return t.ch }

func (t *timer) Stop() {
	c := t.clock
	c.mu.Lock()
	defer c.mu.Unlock()
	c.pending = slices.DeleteFunc(c.pending, func(p *timer) bool { return p == t })
}
