package jupiter

import (
	"cmp"
	"context"
	"slices"
	"sync"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
)

type lane uint8

const (
	swapLane lane = iota
	priceLane
	lanes
)

const (
	paceWindow  = 10 * time.Second
	paceLimit   = 8
	paceReserve = 2
)

type pace struct {
	clock clock.Clock
	limit int

	mu      sync.Mutex
	spent   []time.Time
	waiting [lanes][]*waiter
}

type waiter struct{ granted bool }

func newPace(clk clock.Clock, limit int) *pace {
	return &pace{clock: clk, limit: cmp.Or(limit, paceLimit)}
}

func (p *pace) laneLimit(l lane) int {
	if l == priceLane {
		return max(p.limit-paceReserve, 1)
	}
	return p.limit
}

func (p *pace) take(ctx context.Context, l lane) error {
	w := &waiter{}
	p.mu.Lock()
	defer p.mu.Unlock()
	p.waiting[l] = append(p.waiting[l], w)
	for {
		p.grant()
		if w.granted {
			return nil
		}
		wake := p.clock.After(p.spent[0].Add(paceWindow).Sub(p.clock.Now()))
		p.mu.Unlock()
		select {
		case <-wake:
			p.mu.Lock()
		case <-ctx.Done():
			p.mu.Lock()
			p.waiting[l] = slices.DeleteFunc(p.waiting[l], func(o *waiter) bool { return o == w })
			return errs.Wrap(context.Cause(ctx), errs.CodeUpstreamTimeout, "jupiter.pace")
		}
	}
}

func (p *pace) grant() {
	now := p.clock.Now()
	expired := 0
	for expired < len(p.spent) && !p.spent[expired].Add(paceWindow).After(now) {
		expired++
	}
	p.spent = p.spent[expired:]
	for l := range lanes {
		for len(p.waiting[l]) > 0 && len(p.spent) < p.laneLimit(l) {
			p.waiting[l][0].granted = true
			p.waiting[l] = p.waiting[l][1:]
			p.spent = append(p.spent, now)
		}
	}
}
