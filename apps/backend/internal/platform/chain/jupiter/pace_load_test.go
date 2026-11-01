package jupiter

import (
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/testkit"
)

const (
	loadStep      = 100 * time.Millisecond
	loadHorizon   = 5 * time.Minute
	loadTrades    = 50
	loadTick      = 2 * time.Minute
	loadTickCalls = 26
	quoteTimeout  = 5 * time.Second
	keyPerMinute  = 60
)

type loadCall struct {
	lane    lane
	at      time.Time
	granted time.Time
	w       *waiter
}

type loadProcess struct {
	pace  *pace
	calls []*loadCall
}

func (p *loadProcess) arrive(clk *testkit.Clock, l lane) {
	w := &waiter{}
	p.pace.mu.Lock()
	p.pace.waiting[l] = append(p.pace.waiting[l], w)
	p.pace.mu.Unlock()
	p.calls = append(p.calls, &loadCall{lane: l, at: clk.Now(), w: w})
}

func (p *loadProcess) step(clk *testkit.Clock) {
	p.pace.mu.Lock()
	p.pace.grant()
	p.pace.mu.Unlock()
	for _, c := range p.calls {
		if c.w.granted && c.granted.IsZero() {
			c.granted = clk.Now()
		}
	}
}

type arrival struct {
	at   time.Duration
	l    lane
	proc *loadProcess
}

func loadPlan(api, worker *loadProcess, workerTakesTrades bool) []arrival {
	var plan []arrival
	for i := range loadTrades {
		proc := api
		if workerTakesTrades && i%2 == 1 {
			proc = worker
		}
		at := time.Duration(i) * loadHorizon / loadTrades
		plan = append(plan, arrival{at, swapLane, proc}, arrival{at + time.Second, swapLane, proc})
	}
	for tick := time.Duration(0); tick < loadHorizon; tick += loadTick {
		for range loadTickCalls {
			plan = append(plan, arrival{tick, priceLane, worker})
		}
	}
	return plan
}

func perMinute(grants []time.Time) int {
	busiest := 0
	for _, from := range grants {
		n := 0
		for _, g := range grants {
			if !g.Before(from) && g.Before(from.Add(time.Minute)) {
				n++
			}
		}
		busiest = max(busiest, n)
	}
	return busiest
}

func simulate(t *testing.T, apiLimit, workerLimit int, workerTakesTrades bool) (time.Duration, int) {
	t.Helper()
	start := time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)
	clk := testkit.NewClock(start)
	api := &loadProcess{pace: newPace(clk, apiLimit)}
	worker := &loadProcess{pace: newPace(clk, workerLimit)}
	plan := loadPlan(api, worker, workerTakesTrades)
	for elapsed := time.Duration(0); elapsed <= loadHorizon+time.Minute; elapsed += loadStep {
		for _, a := range plan {
			if a.at == elapsed {
				a.proc.arrive(clk, a.l)
			}
		}
		api.step(clk)
		worker.step(clk)
		clk.Advance(loadStep)
	}
	var grants []time.Time
	var maxSwapWait time.Duration
	for _, proc := range []*loadProcess{api, worker} {
		for _, c := range proc.calls {
			if c.granted.IsZero() {
				t.Fatalf("a %d-lane call at %v was never granted", c.lane, c.at.Sub(start))
			}
			grants = append(grants, c.granted)
			if c.lane == swapLane {
				maxSwapWait = max(maxSwapWait, c.granted.Sub(c.at))
			}
		}
	}
	return maxSwapWait, perMinute(grants)
}

func TestPaceLoad_fiftyTradesAndThreePriceTicksFitTheFreeKeyWithNoQuoteTimeout(t *testing.T) {
	t.Parallel()
	for name, tc := range map[string]struct {
		api, worker int
		split       bool
	}{
		"api takes the trades":            {5, 5, false},
		"api and worker share the trades": {5, 5, true},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			wait, busiest := simulate(t, tc.api, tc.worker, tc.split)
			if wait >= quoteTimeout {
				t.Fatalf(
					"slowest trade call waited %v for the limiter, want under the %v quote timeout",
					wait,
					quoteTimeout,
				)
			}
			if busiest > keyPerMinute {
				t.Fatalf("busiest 60 s sent %d calls on the key, want at most %d", busiest, keyPerMinute)
			}
			t.Logf("%s: slowest trade call waited %v, busiest minute %d of %d calls", name, wait, busiest, keyPerMinute)
		})
	}
}

func TestPaceLoad_theWorkersPriceLaneStillCoversTheListedMints(t *testing.T) {
	t.Parallel()
	const (
		listedMints = 1500
		fetchWindow = 105 * time.Second
	)
	c := &Client{pace: newPace(testkit.NewClock(time.Date(2026, 11, 1, 12, 0, 0, 0, time.UTC)), 6)}
	if got := c.PriceCapacity(fetchWindow); got < listedMints {
		t.Fatalf(
			"a worker limit of 6 prices %d mints in %v, want at least the %d listed",
			got,
			fetchWindow,
			listedMints,
		)
	}
}
