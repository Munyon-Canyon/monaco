package verify

import (
	"context"
	"net/http"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"go.opentelemetry.io/otel/metric/noop"

	"github.com/monaco/monaco/apps/backend/internal/errs"
	"github.com/monaco/monaco/apps/backend/internal/events"
	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/clock"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/db"
	"github.com/monaco/monaco/apps/backend/internal/platform/httpx/api"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/platform/observability"
	"github.com/monaco/monaco/apps/backend/internal/platform/poller"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const fixtureFlows = tools.Header + "\n" +
	"95\tPrices\tfixture\tpoller:fixture.prices\tTickPrices\t\t\tok;UpstreamUnavailable\tbuilt\tdocs/x.md#prices\n" +
	"96\tEcho\tsystem\tconsumer:system.pinged\tEcho\t\tfixture.ok\tok;InvalidInput\tbuilt\tdocs/x.md#echo\n" +
	"97\tVote\tsystem\tPOST /v1/system/pings\tRecordPing\t\t\tok;InvalidInput\tbuilt\tdocs/x.md#vote\n"

func awaitPrices(s *scenario.Scenario) { s.When(scenario.AwaitTick("fixture.prices")) }

func fixtureScripts() map[string]flows.Script {
	ping := func(s *scenario.Scenario) {
		s.Given(scenario.AsUser("alice")).
			When(scenario.Post("/v1/system/pings", `{"note":"hi"}`), scenario.ExpectStatus(http.StatusCreated)).
			Then(scenario.EventuallyEvent(events.TypeSystemPinged))
	}
	return map[string]flows.Script{
		"F95TickPricesOK": func(s *scenario.Scenario) {
			awaitPrices(s)
			s.Then(scenario.ExpectTick("fixture.prices", 3, 2))
		},
		"F95TickPricesUpstreamUnavailable": awaitPrices,
		"F96EchoOK":                        ping,
		"F96EchoInvalidInput":              ping,
		"F97RecordPingOK":                  ping,
		"F97RecordPingInvalidInput":        ping,
	}
}

func TestRouteMismatch_judgesACodeOutcomeByTheLastCallOnTheRoute(t *testing.T) {
	t.Parallel()
	const route = "POST /v1/system/pings"
	call := func(status int, body string) scenario.Exchange {
		return scenario.Exchange{
			Method: http.MethodPost, Path: "/v1/system/pings", Status: status, Response: []byte(body),
		}
	}
	ok, bad := call(http.StatusCreated, `{}`), call(http.StatusBadRequest, `{"code":"invalid_input"}`)
	for _, tc := range []struct {
		name      string
		outcome   string
		exchanges []scenario.Exchange
		wantEmpty bool
	}{
		{"setup call then the failing call", "InvalidInput", []scenario.Exchange{ok, bad}, true},
		{"no call fails", "InvalidInput", []scenario.Exchange{ok, ok}, false},
		{"setup call fails but the final call does not", "InvalidInput", []scenario.Exchange{bad, ok}, false},
		{"success outcome with a later failure", "ok", []scenario.Exchange{ok, bad}, true},
		{"success outcome whose first call fails", "ok", []scenario.Exchange{bad, ok}, false},
	} {
		res := &Result{Unit: fixtureUnit(t, "97", tc.outcome), Exchanges: tc.exchanges}
		if got := routeMismatch(res, route); (got == "") != tc.wantEmpty {
			t.Errorf("%s: routeMismatch = %q, want empty %v", tc.name, got, tc.wantEmpty)
		}
	}
}

func fixtureUnit(t *testing.T, flow, outcome string) Unit {
	t.Helper()
	all, problems := tools.Parse("fixture.tsv", strings.NewReader(fixtureFlows))
	if len(problems) > 0 {
		t.Fatalf("fixture flows: %v", problems)
	}
	units, err := selectUnits(all, Target{Flow: flow, Outcome: outcome}, fixtureScripts())
	if err != nil || len(units) != 1 {
		t.Fatalf("selectUnits(%s %s) = %d units, %v", flow, outcome, len(units), err)
	}
	return units[0]
}

func TestOutcomeMismatch_readsTheTriggerLineOfTheKindAndOutcomeWrittenAfterTheScriptStarted(t *testing.T) {
	t.Parallel()
	const (
		tick   = `{"msg":"poller.tick","poller":"fixture.prices","scanned":1,"changed":1,"duration_ms":1}`
		failed = `{"msg":"poller.tick.failed","poller":"fixture.prices","code":"upstream_unavailable"}`
		acked  = `{"msg":"bus.dispatched","subject":"ns.events.system.pinged","outcome":"ack","code":"ok"}`
		bare   = `{"msg":"bus.dispatched","subject":"events.system.pinged","outcome":"ack","code":"ok"}`
		termed = `{"msg":"bus.dispatched","subject":"ns.events.system.pinged","outcome":"term","code":"invalid_input"}`

		noTick   = "no poller.tick for fixture.prices after the script started"
		noFailed = "no poller.tick.failed for fixture.prices with code upstream_unavailable after the script started"
		noAck    = "no bus.dispatched for system.pinged with outcome ack after the script started"
		noCode   = "no bus.dispatched for system.pinged with code invalid_input after the script started"
	)
	d := &driver{clock: clock.Real{}, env: Env{Subject: func(s string) string { return "ns." + s }}}
	for _, tc := range []struct {
		flow, outcome string
		before, after []string
		want          string
	}{
		{"95", "ok", nil, []string{failed, tick}, ""},
		{"95", "ok", []string{tick}, []string{failed}, noTick},
		{"95", "UpstreamUnavailable", nil, []string{tick, failed}, ""},
		{"95", "UpstreamUnavailable", []string{failed}, []string{tick}, noFailed},
		{"96", "ok", nil, []string{termed, acked}, ""},
		{"96", "ok", []string{acked}, []string{termed}, noAck},
		{"96", "ok", nil, []string{bare}, noAck},
		{"96", "InvalidInput", nil, []string{acked, termed}, ""},
		{"96", "InvalidInput", []string{termed}, []string{acked}, noCode},
	} {
		d.env.Logs = &Logs{}
		for _, line := range tc.before {
			d.env.Logs.add(procWorker, line)
		}
		res := &Result{Unit: fixtureUnit(t, tc.flow, tc.outcome), logFrom: d.env.Logs.mark()}
		for _, line := range tc.after {
			d.env.Logs.add(procWorker, line)
		}
		if got := d.outcomeMismatch(res); got != tc.want {
			t.Errorf("flow %s %s with %q then %q = %q, want %q", tc.flow, tc.outcome, tc.before, tc.after, got, tc.want)
		}
	}
}

type fixturePoller struct {
	every  time.Duration
	report poller.Report
	err    error
}

func (fixturePoller) Name() string { return "fixture.prices" }

func (p fixturePoller) Interval() time.Duration { return p.every }

func (p fixturePoller) Tick(context.Context) (poller.Report, error) { return p.report, p.err }

func pollerEnv(t *testing.T) Env {
	t.Helper()
	env := unservedEnv("")
	env.Pool = testkit.DB(t)
	return env
}

func tickAsTheWorker(t *testing.T, env Env, p poller.Poller) {
	t.Helper()
	runner, err := poller.NewRunner(env.Pool, clock.Real{}, noop.NewMeterProvider().Meter("verify"))
	if err != nil {
		t.Fatal(err)
	}
	logger := observability.NewLogger(config.Config{Env: config.EnvTest},
		&lineWriter{line: func(s string) { env.Logs.add(procWorker, s) }})
	ctx, cancel := context.WithCancel(observability.WithLogger(t.Context(), logger))
	var wg sync.WaitGroup
	wg.Go(func() { _ = runner.Run(ctx, p) })
	t.Cleanup(func() {
		cancel()
		wg.Wait()
	})
	testkit.Eventually(t, func() bool {
		return slices.ContainsFunc(env.Logs.Lines(), func(l Line) bool {
			return strings.Contains(l.Text, `"poller":"fixture.prices"`)
		})
	}, 5*time.Second)
}

const virtualConverge = time.Second

func failingConverge(b Budget) Budget {
	b.Converge = virtualConverge
	return b
}

func runUnit(t *testing.T, env Env, budget Budget, u Unit) *Result {
	t.Helper()
	d, err := newDriver(env, budget)
	if err != nil {
		t.Fatal(err)
	}
	if budget.Converge == virtualConverge {
		return runPastConverge(t, d, u)
	}
	return d.run(t.Context(), u)
}

func TestVerify_aPollerFlowPassesOnATickAndNamesThePollerWhenNoneComes(t *testing.T) {
	t.Parallel()
	short := DefaultBudget()
	short.Flow = 300 * time.Millisecond
	for _, tc := range []struct {
		name   string
		every  time.Duration
		budget Budget
		script flows.Script
		want   string
		over   bool
	}{
		{"ticks", 20 * time.Millisecond, DefaultBudget(), nil, "", false},
		{
			"no tick in the budget", time.Hour, short, nil, "scenario: a tick of poller fixture.prices after the step " +
				"started did not happen: over budget: flow 95 ok flow took longer than 300ms", true,
		},
		{
			"script never waits", time.Hour, failingConverge(DefaultBudget()), func(s *scenario.Scenario) { s.When() },
			"flow 95 ok invariant: no poller.tick for fixture.prices after the script started", false,
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := pollerEnv(t)
			tickAsTheWorker(t, env, fixturePoller{every: tc.every, report: poller.Report{Scanned: 3, Changed: 2}})
			u := fixtureUnit(t, "95", "ok")
			if tc.script != nil {
				u.Script = tc.script
			}
			res := runUnit(t, env, tc.budget, u)
			if res.Failure != tc.want || (res.Over != nil) != tc.over || tc.over && res.Over.Phase != PhaseFlow {
				t.Fatalf("flow 95 ok = %q over budget %+v, want %q over the flow budget %v",
					res.Failure, res.Over, tc.want, tc.over)
			}
			if tc.want == "" && (len(res.logLines) != 1 || !strings.HasPrefix(res.logLines[0], procWorker+": ") ||
				!strings.Contains(res.logLines[0], `"msg":"poller.tick"`) ||
				!strings.Contains(res.logLines[0], `"poller":"fixture.prices"`)) {
				t.Fatalf("evidence log lines = %q, want the worker's poller.tick for fixture.prices", res.logLines)
			}
		})
	}
}

func TestVerify_aPollerCodeOutcomePassesOnlyOnAFailedTickWithThatCode(t *testing.T) {
	t.Parallel()
	unavailable := errs.New(errs.CodeUpstreamUnavailable, "fixture.tick")
	const noFailedTick = "flow 95 UpstreamUnavailable invariant: no poller.tick.failed for fixture.prices " +
		"with code upstream_unavailable after the script started"
	for _, tc := range []struct {
		name, outcome string
		err           error
		want          string
	}{
		{"failed tick", "UpstreamUnavailable", unavailable, ""},
		{"good tick", "UpstreamUnavailable", nil, noFailedTick},
		{"other code", "UpstreamUnavailable", errs.New(errs.CodeUpstreamTimeout, "fixture.tick"), noFailedTick},
		{
			"ok on a failed tick", "ok", unavailable,
			"flow 95 ok invariant: no poller.tick for fixture.prices after the script started",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := pollerEnv(t)
			tickAsTheWorker(t, env, fixturePoller{every: 20 * time.Millisecond, err: tc.err})
			u := fixtureUnit(t, "95", tc.outcome)
			u.Script = awaitPrices
			budget := DefaultBudget()
			if tc.want != "" {
				budget = failingConverge(budget)
			}
			if res := runUnit(t, env, budget, u); res.Failure != tc.want {
				t.Fatalf("flow 95 %s failure = %q, want %q", tc.outcome, res.Failure, tc.want)
			}
		})
	}
}

type fixtureConsumers struct{ handlers []bus.HandlerSpec }

func (fixtureConsumers) Name() string { return "fixture" }

func (fixtureConsumers) Mount(api.Mount) {}

func (m fixtureConsumers) Consumers() []bus.Consumer {
	return []bus.Consumer{{Durable: "verify_fixture", Handlers: m.handlers}}
}

func (fixtureConsumers) Pollers() []poller.Poller { return nil }

type routesOnly struct{ *system.Module }

func (routesOnly) Consumers() []bus.Consumer { return nil }

func handles(name string, err error) bus.HandlerSpec {
	return bus.Handle(name, func(context.Context, db.Tx, events.SystemPinged, time.Time) error { return err })
}

func TestVerify_aConsumerFlowPassesOnItsDispatchAndFailsWhenTheHandlerNeverRuns(t *testing.T) {
	t.Parallel()
	pings := func(d module.Deps) module.Module { return system.New(d) }
	fixture := func(handlers ...bus.HandlerSpec) func(module.Deps) module.Module {
		return func(module.Deps) module.Module { return fixtureConsumers{handlers: handlers} }
	}
	acks := fixture(handles("fixture.ok", nil))
	for _, tc := range []struct {
		name    string
		outcome string
		mods    []func(module.Deps) module.Module
		want    string
	}{
		{"acked", "ok", []func(module.Deps) module.Module{pings, acks}, ""},
		{
			"handler never runs", "ok",
			[]func(module.Deps) module.Module{func(d module.Deps) module.Module { return routesOnly{system.New(d)} }},
			"flow 96 ok invariant: no bus.dispatched for system.pinged with outcome ack after the script started",
		},
		{
			"code", "InvalidInput", []func(module.Deps) module.Module{
				pings, fixture(handles("fixture.ok", nil), handles("fixture.fail", errs.New(errs.CodeInvalidInput, "fixture"))),
			}, "",
		},
		{
			"no code", "InvalidInput",
			[]func(module.Deps) module.Module{pings, acks},
			"flow 96 InvalidInput invariant: no bus.dispatched for system.pinged with code invalid_input " +
				"after the script started",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := servedEnvWith(t, tc.mods...)
			budget := DefaultBudget()
			if tc.want != "" {
				budget = failingConverge(budget)
			}
			u := fixtureUnit(t, "96", tc.outcome)
			if res := runUnit(t, env, budget, u); res.Failure != tc.want {
				t.Fatalf("flow 96 %s failure = %q, want %q", tc.outcome, res.Failure, tc.want)
			}
		})
	}
}
