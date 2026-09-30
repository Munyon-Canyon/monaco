package scenario

import (
	"context"
	"fmt"
	"runtime"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
)

type recordingT struct {
	*testing.T
	ctx    func() context.Context
	failed string
}

func (r *recordingT) Context() context.Context { return r.ctx() }

func (r *recordingT) Fatal(args ...any) { r.fail(fmt.Sprint(args...)) }

func (r *recordingT) Fatalf(format string, args ...any) { r.fail(fmt.Sprintf(format, args...)) }

func (r *recordingT) fail(msg string) {
	r.failed = msg
	runtime.Goexit()
}

func failure(t *testing.T, ctx func() context.Context, open func(T) *Scenario, steps ...Step) string {
	t.Helper()
	r := &recordingT{T: t, ctx: ctx}
	var wg sync.WaitGroup
	wg.Go(func() { open(r).When(steps...) })
	wg.Wait()
	return r.failed
}

type fixtureModule struct{ pollers []poller.Poller }

func (fixtureModule) Name() string { return "fixture" }

func (fixtureModule) Routes(*httpx.Routes) {}

func (fixtureModule) Consumers() []bus.Consumer { return nil }

func (m fixtureModule) Pollers() []poller.Poller { return m.pollers }

type fixturePoller struct {
	name   string
	report poller.Report
	err    error
	ticks  *atomic.Int32
}

func (p fixturePoller) Name() string { return p.name }

func (fixturePoller) Interval() time.Duration { return time.Hour }

func (p fixturePoller) Tick(context.Context) (poller.Report, error) {
	p.ticks.Add(1)
	return p.report, p.err
}

func TestAwaitTick_inProcessTicksThePollerOnceAndExpectTickChecksWhatItFound(t *testing.T) {
	t.Parallel()
	var prices, failing atomic.Int32
	a := start(t, options{modules: []func(module.Deps) module.Module{func(module.Deps) module.Module {
		return fixtureModule{pollers: []poller.Poller{
			fixturePoller{name: "fixture.prices", report: poller.Report{Scanned: 3, Changed: 2}, ticks: &prices},
			fixturePoller{
				name: "fixture.failing", err: errs.New(errs.CodeUpstreamUnavailable, "fixture.tick"), ticks: &failing,
			},
		}}
	}}})
	inProcess := func(r T) *Scenario { return newScenario(r, a.backend()) }
	for _, tc := range []struct {
		name  string
		steps []Step
		want  string
	}{
		{"counts", []Step{AwaitTick("fixture.prices"), ExpectTick("fixture.prices", 3, 2)}, ""},
		{
			"wrong counts",
			[]Step{AwaitTick("fixture.prices"), ExpectTick("fixture.prices", 3, 1)},
			"scenario: poller fixture.prices scanned 3 changed 2, want scanned 3 changed 1",
		},
		{
			"failed tick",
			[]Step{AwaitTick("fixture.failing"), ExpectTick("fixture.failing", 0, 0)},
			"scenario: poller fixture.failing failed with code upstream_unavailable, want scanned 0 changed 0",
		},
		{"unknown poller", []Step{AwaitTick("fixture.ghost")}, "scenario: no module registers poller fixture.ghost"},
		{
			"no await",
			[]Step{ExpectTick("fixture.prices", 3, 2)},
			"scenario: ExpectTick(fixture.prices) needs AwaitTick(fixture.prices) first",
		},
	} {
		if got := failure(t, t.Context, inProcess, tc.steps...); got != tc.want {
			t.Errorf("%s: failure = %q, want %q", tc.name, got, tc.want)
		}
	}
	if prices.Load() != 2 || failing.Load() != 1 {
		t.Fatalf("ticks = %d prices, %d failing, want one per AwaitTick: 2 and 1", prices.Load(), failing.Load())
	}
}

func TestAwaitTick_againstAStackWaitsForATickWrittenAfterTheStepStarted(t *testing.T) {
	t.Parallel()
	const (
		before = `{"msg":"poller.tick","poller":"market.prices","scanned":9,"changed":9,"duration_ms":1}`
		after  = `{"msg":"poller.tick.skipped_locked","poller":"market.prices"}` + "\n" +
			`{"msg":"poller.tick","poller":"funding.deposits","scanned":5,"changed":5,"duration_ms":1}` + "\n" +
			`{"msg":"poller.tick","poller":"market.prices","scanned":2,"changed":1,"duration_ms":1}`
	)
	stack := &lineLog{note: newNotifier()}
	_, _ = fmt.Fprintln(stack, before)
	remote := func(marked chan<- struct{}) func(T) *Scenario {
		var once sync.Once
		return func(r T) *Scenario {
			return Against(r, Remote{Enter: func(Stage) {}, Logs: func(from int) ([]string, <-chan struct{}) {
				defer once.Do(func() { close(marked) })
				return stack.since(from)
			}})
		}
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	got := failure(t, func() context.Context { return cancelled }, remote(make(chan struct{})),
		AwaitTick("market.prices"))
	if want := "scenario: a tick of poller market.prices after the step started did not happen: " +
		"context canceled"; got != want {
		t.Fatalf("AwaitTick with only an earlier tick = %q, want %q", got, want)
	}
	marked := make(chan struct{})
	var wg sync.WaitGroup
	wg.Go(func() {
		<-marked
		_, _ = fmt.Fprintln(stack, after)
	})
	got = failure(t, t.Context, remote(marked), AwaitTick("market.prices"), ExpectTick("market.prices", 2, 1))
	wg.Wait()
	if got != "" {
		t.Fatalf("AwaitTick then ExpectTick against the stack = %q, want a pass", got)
	}
}
