package chaos

import (
	"context"
	"fmt"
	"math/rand/v2"
	"slices"
	"strings"
	"testing"
	"time"

	"github.com/nats-io/nats.go/jetstream"

	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
)

const (
	window      = 8
	maxInjected = 3
	fateSides   = 5
)

type Seed uint64

type fate uint8

const (
	deliver fate = iota
	redeliver
	crashBeforeCommit
)

func (f fate) String() string {
	return [...]string{"deliver", "redeliver", "crash-before-commit"}[f]
}

type Result struct {
	Verdicts map[string]Verdict
	Trace    []string
}

func (r Result) Terms() []string {
	var out []string
	for id, v := range r.Verdicts {
		if v.Outcome == bus.OutcomeTerm {
			out = append(out, id+" "+v.Reason)
		}
	}
	slices.Sort(out)
	return out
}

type dispatcher interface {
	Dispatch(ctx context.Context, durable string, msg jetstream.Msg)
}

type delivery struct {
	msg    *Msg
	due    time.Duration
	passed int
}

type run struct {
	t        testing.TB
	reg      dispatcher
	consumer bus.Consumer
	rng      *rand.Rand
	window   int
	now      time.Duration
	queue    []delivery
	injected map[string]int
	result   Result
}

func Dispatch(
	ctx context.Context, tb testing.TB, seed Seed, reg *bus.Registry, c bus.Consumer, msgs []*Msg,
) Result {
	tb.Helper()
	if !faultpoint.Enabled {
		tb.Fatal("chaos: built without -tags faultpoints, so no handler can crash before commit")
	}
	r := newRun(tb, reg, c, msgs)
	r.rng = newRNG(seed)
	r.window = window
	return r.drain(ctx)
}

func newRNG(seed Seed) *rand.Rand { return rand.New(rand.NewPCG(uint64(seed), 0)) }

func Baseline(ctx context.Context, tb testing.TB, reg *bus.Registry, c bus.Consumer, msgs []*Msg) Result {
	tb.Helper()
	r := newRun(tb, reg, c, msgs)
	r.window = 1
	res := r.drain(ctx)
	baselineCheck(tb, res)
	return res
}

func baselineCheck(tb testing.TB, res Result) {
	tb.Helper()
	if terms := res.Terms(); len(terms) > 0 {
		tb.Fatalf(
			"chaos: the in-order run termed, so the consumer cannot handle its own generated events:\n%s",
			strings.Join(terms, "\n"),
		)
	}
}

func newRun(tb testing.TB, reg dispatcher, c bus.Consumer, msgs []*Msg) *run {
	tb.Helper()
	r := &run{
		t: tb, reg: reg, consumer: c,
		injected: map[string]int{},
		result:   Result{Verdicts: make(map[string]Verdict, len(msgs))},
	}
	for _, m := range msgs {
		m.delivered, m.verdict = 0, Verdict{}
		r.queue = append(r.queue, delivery{msg: m})
	}
	return r
}

func (r *run) drain(ctx context.Context) Result {
	r.t.Helper()
	for len(r.queue) > 0 {
		i := r.pick()
		m := r.queue[i].msg
		r.queue = slices.Delete(r.queue, i, i+1)
		r.attempt(ctx, m)
	}
	return r.result
}

func (r *run) pick() int {
	due := make([]int, 0, r.window)
	next := 0
	for i, d := range r.queue {
		if d.due <= r.now {
			if due = append(due, i); len(due) == r.window {
				break
			}
		}
		if d.due < r.queue[next].due {
			next = i
		}
	}
	switch {
	case len(due) == 0:
		r.now = r.queue[next].due
		return next
	case r.rng == nil || r.queue[due[0]].passed >= r.window-1:
		return due[0]
	}
	choice := due[r.rng.IntN(len(due))]
	for _, i := range due {
		if i < choice {
			r.queue[i].passed++
		}
	}
	return choice
}

func (r *run) attempt(ctx context.Context, m *Msg) {
	r.t.Helper()
	f, k := r.roll(m)
	m.delivered++
	m.verdict = Verdict{}
	if f == crashBeforeCommit {
		ctx = faultpoint.ArmedAfter(ctx, faultpoint.BeforeCommit, k)
	}
	crashed := r.dispatch(ctx, m)
	r.settle(m, f, k, crashed)
}

func (r *run) roll(m *Msg) (fate, int) {
	if r.rng == nil || r.injected[m.id] >= maxInjected {
		return deliver, 0
	}
	routed := r.routed(m)
	switch side := r.rng.IntN(fateSides); {
	case side == fateSides-1 && routed > 0:
		return crashBeforeCommit, r.rng.IntN(routed)
	case side >= fateSides-2:
		return redeliver, 0
	default:
		return deliver, 0
	}
}

func (r *run) routed(m *Msg) int {
	n := 0
	for _, h := range r.consumer.Handlers {
		if h.Type() == m.typ {
			n++
		}
	}
	return n
}

func (r *run) dispatch(ctx context.Context, m *Msg) (crashed bool) {
	defer func() {
		if p := recover(); p != nil {
			if !faultpoint.IsCrash(p) {
				panic(p)
			}
			crashed = true
		}
	}()
	r.reg.Dispatch(ctx, r.consumer.Durable, m)
	return false
}

func (r *run) settle(m *Msg, f fate, k int, crashed bool) {
	r.t.Helper()
	v := m.verdict
	switch {
	case crashed:
		r.injected[m.id]++
		r.requeue(m, f, fmt.Sprintf("crashed before commit %d, redelivered", k+1), 0)
	case v.Outcome == bus.OutcomeAck && f == redeliver:
		r.injected[m.id]++
		r.requeue(m, f, "ack dropped, redelivered", 0)
	case v.Outcome == bus.OutcomeNak:
		if m.delivered >= bus.MaxDeliver {
			r.t.Fatalf("chaos: %s naked at delivery %d, the consumer's MaxDeliver; "+
				"chaos does not simulate the max-deliveries advisory\n%s", m.id, m.delivered, r.trace())
		}
		r.requeue(m, f, "nak "+v.Reason+" -> due "+(r.now+v.Delay).String(), v.Delay)
	case v.Outcome == bus.OutcomeAck || v.Outcome == bus.OutcomeTerm:
		r.result.Verdicts[m.id] = v
		r.log(m, f, string(v.Outcome)+" "+v.Reason)
	default:
		r.t.Fatalf("chaos: dispatch of %s returned without a verdict\n%s", m.id, r.trace())
	}
}

func (r *run) requeue(m *Msg, f fate, note string, after time.Duration) {
	r.log(m, f, note)
	r.queue = append(r.queue, delivery{msg: m, due: r.now + after})
}

func (r *run) log(m *Msg, f fate, note string) {
	r.result.Trace = append(r.result.Trace,
		fmt.Sprintf("t=%s %s #%d %s: %s", r.now, m.id, m.delivered, f, strings.TrimSpace(note)))
}

func (r *run) trace() string { return strings.Join(r.result.Trace, "\n") }
