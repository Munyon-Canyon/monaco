package verify

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"golang.org/x/sync/errgroup"

	"github.com/monaco/monaco/apps/backend/internal/modules/identity"
	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/config"
	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
	"github.com/monaco/monaco/apps/backend/internal/testkit/fakes"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func servedEnv(t *testing.T) Env {
	t.Helper()
	return servedEnvWith(t, func(d module.Deps) module.Module { return system.New(d) })
}

func servedEnvWith(t *testing.T, mods ...func(module.Deps) module.Module) Env {
	t.Helper()
	logs := &Logs{}
	sv := scenario.Serve(t, scenario.WithContract(contract),
		scenario.WithModules(mods...),
		scenario.WithLogs(&lineWriter{line: func(s string) { logs.add("app", s) }}))
	return Env{
		API: sv.URL, TokenKey: sv.TokenKey, Pool: sv.Pool, JS: sv.JS, Events: sv.Events,
		DeadLetter: sv.DeadLetter, Subject: sv.Subject, Consumers: sv.Consumers, Logs: logs,
		Arm: func(context.Context, Unit) error { return nil },
	}
}

func servedIdentityEnv(t *testing.T) Env {
	t.Helper()
	upstream := httptest.NewServer(fakes.New())
	t.Cleanup(upstream.Close)
	cfg := config.Config{
		Privy: config.Privy{
			AppID: PrivyAppID, BaseURL: upstream.URL + "/privy", VerificationKey: fakes.PrivyVerificationKey(),
			AuthorizationKeyID: fakes.PrivyAuthorizationKeyID,
		},
		Timeouts: config.Timeouts{Privy: 10 * time.Second},
	}
	logs := &Logs{}
	sv := scenario.Serve(t, scenario.WithContract(contract),
		scenario.WithModules(func(d module.Deps) module.Module {
			d.Config = cfg
			return identity.New(d)
		}),
		scenario.WithLogs(&lineWriter{line: func(s string) { logs.add("app", s) }}))
	return Env{
		API: sv.URL, Fakes: upstream.URL, PrivyAppID: PrivyAppID, TokenKey: sv.TokenKey, Pool: sv.Pool, JS: sv.JS,
		Events: sv.Events, DeadLetter: sv.DeadLetter, Consumers: sv.Consumers, Logs: logs,
		Arm: func(context.Context, Unit) error { return nil },
	}
}

func unservedEnv(api string) Env {
	return Env{
		API: api, TokenKey: "verify-unserved", Logs: &Logs{}, Arm: func(context.Context, Unit) error { return nil },
	}
}

func TestDriverSubscribeCoreReceivesThePublishedMessage(t *testing.T) {
	t.Parallel()
	nats, err := testkit.StartEmbeddedNATS()
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(nats.Stop)
	d := &driver{env: Env{NATS: nats, Subject: func(subject string) string { return "verify." + subject }}}
	messages := d.subscribeCore(t, "price.tick")
	publisher, err := nats.Connect("verify-test-publisher")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(publisher.Close)
	if err := publisher.Publish("verify.price.tick", []byte(`{"v":1}`)); err != nil {
		t.Fatal(err)
	}
	if err := publisher.Flush(); err != nil {
		t.Fatal(err)
	}
	var got []byte
	testkit.Eventually(t, func() bool {
		select {
		case got = <-messages:
			return true
		default:
			return false
		}
	}, time.Second)
	if string(got) != `{"v":1}` {
		t.Fatalf("message = %s", got)
	}
}

func TestDriverSubscribeCoreNamesSetupFailures(t *testing.T) {
	t.Parallel()
	t.Run("connect", func(t *testing.T) {
		t.Parallel()
		d := &driver{env: Env{
			NATS:    &testkit.EmbeddedNATS{URL: "nats://127.0.0.1:1"},
			Subject: func(subject string) string { return subject },
		}}
		gotErr := subscribeCoreFailure(t, d, "price.tick")
		if gotErr == nil || !strings.Contains(gotErr.Error(), "connect core") {
			t.Fatalf("error = %v", gotErr)
		}
	})
	t.Run("subscribe", func(t *testing.T) {
		t.Parallel()
		nats, err := testkit.StartEmbeddedNATS()
		if err != nil {
			t.Fatal(err)
		}
		t.Cleanup(nats.Stop)
		d := &driver{env: Env{NATS: nats, Subject: func(string) string { return "bad subject" }}}
		gotErr := subscribeCoreFailure(t, d, "price.tick")
		if gotErr == nil || !strings.Contains(gotErr.Error(), "subscribe core") {
			t.Fatalf("error = %v", gotErr)
		}
	})
	t.Run("flush", func(t *testing.T) {
		t.Parallel()
		nats, err := testkit.StartEmbeddedNATS()
		if err != nil {
			t.Fatal(err)
		}
		d := &driver{env: Env{
			NATS:            nats,
			Subject:         func(subject string) string { return subject },
			BeforeCoreFlush: nats.Stop,
		}}
		gotErr := subscribeCoreFailure(t, d, "price.tick")
		if gotErr == nil || !strings.Contains(gotErr.Error(), "flush core") {
			t.Fatalf("error = %v", gotErr)
		}
	})
}

func subscribeCoreFailure(t *testing.T, d *driver, subject string) error {
	t.Helper()
	flow := &flowT{ctx: t.Context}
	done := make(chan struct{})
	go func() {
		defer close(done)
		d.subscribeCore(flow, subject)
	}()
	<-done
	flow.runCleanups()
	return flow.err()
}

func flow01(t *testing.T, target Target) []Unit {
	t.Helper()
	all, err := readFlows(backendDir(t))
	if err != nil {
		t.Fatal(err)
	}
	units, err := selectUnits(all, target, flows.Scripts())
	if err != nil {
		t.Fatal(err)
	}
	return units
}

func TestVerifyUnits_flow01PassesOverHTTPAgainstThePrivyFakes(t *testing.T) {
	t.Parallel()
	env := servedIdentityEnv(t)
	for name, tc := range map[string]struct {
		target Target
		want   []string
	}{
		"every outcome": {
			Target{Flow: "01"}, []string{"PASS flow 01 ok (", "PASS flow 01 AccountDeleted", "PASS flow 01 PrivyUnavailable"},
		},
		"crash before commit": {Target{Flow: "01", CrashAt: "before-commit"}, []string{"PASS flow 01 crash:before-commit"}},
	} {
		t.Run(name, func(t *testing.T) {
			t.Parallel()
			cfg, out := driveConfig(DefaultBudget())
			if err := verifyUnits(
				t.Context(),
				cfg,
				env,
				newReport(t.Context(), tc.target, flow01(t, tc.target)),
				parallelFlows,
			); err != nil {
				t.Fatalf("verifyUnits: %v\n%s", err, out)
			}
			for _, want := range tc.want {
				if !strings.Contains(out.String(), want) {
					t.Errorf("output lacks %q:\n%s", want, out)
				}
			}
		})
	}
}

func flow00(t *testing.T, target Target) []Unit {
	t.Helper()
	all, err := readFlows(backendDir(t))
	if err != nil {
		t.Fatal(err)
	}
	units, err := selectUnits(all, target, flows.Scripts())
	if err != nil {
		t.Fatal(err)
	}
	return units
}

func driveConfig(budget Budget) (Config, *bytes.Buffer) {
	var out bytes.Buffer
	return Config{Budget: budget, Stdout: &out, Stderr: &out}, &out
}

func TestVerifyUnits_flow00PassesOverHTTPWithAuthIdempotencyKeysAndSSE(t *testing.T) {
	t.Parallel()
	cfg, out := driveConfig(DefaultBudget())
	if err := verifyUnits(
		t.Context(),
		cfg,
		servedEnv(t),
		newReport(t.Context(), Target{}, flow00(t, Target{Flow: "00"})),
		parallelFlows,
	); err != nil {
		t.Fatalf("verifyUnits: %v\n%s", err, out)
	}
	for _, want := range []string{"PASS flow 00 ok (seed ", "PASS flow 00 InvalidInput", "PASS flow 00 Unauthorized"} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestDriver_restartsTheScopedProcessAfterACrashResponse(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"faultpoint"}`))
	}))
	t.Cleanup(api.Close)
	var got struct {
		flow  string
		point faultpoint.Name
		calls int
	}
	d, err := newDriver(Env{
		API: api.URL, TokenKey: "verify-restart", Logs: &Logs{},
		Crash: func(_ context.Context, u Unit, point faultpoint.Name) error {
			got.flow, got.point, got.calls = u.Flow.ID, point, got.calls+1
			return nil
		},
	}, DefaultBudget())
	if err != nil {
		t.Fatal(err)
	}
	u := Unit{
		Flow:    tools.Flow{ID: "01", Trigger: "GET /restart"},
		Outcome: "crash:before-commit",
		Script: func(s *scenario.Scenario) {
			s.When(scenario.Get("/restart"), scenario.ExpectStatus(http.StatusServiceUnavailable))
		},
	}
	if err := d.script(t.Context(), u, &Result{Unit: u, Phases: map[Phase]time.Duration{}}); err != nil {
		t.Fatalf("script = %v", err)
	}
	if got.flow != "01" || got.point != faultpoint.BeforeCommit || got.calls != 1 {
		t.Fatalf("restart = %+v, want flow 01, before-commit, once", got)
	}
}

func TestDriver_rejectsAnUnexpectedServiceUnavailableResponse(t *testing.T) {
	t.Parallel()
	api := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/problem+json")
		w.WriteHeader(http.StatusServiceUnavailable)
		_, _ = w.Write([]byte(`{"code":"upstream_unavailable"}`))
	}))
	t.Cleanup(api.Close)
	var calls int
	d, err := newDriver(Env{
		API: api.URL, TokenKey: "verify-restart", Logs: &Logs{},
		Crash: func(context.Context, Unit, faultpoint.Name) error { calls++; return nil },
	}, DefaultBudget())
	if err != nil {
		t.Fatal(err)
	}
	u := Unit{
		Flow:    tools.Flow{ID: "01", Trigger: "GET /restart"},
		Outcome: "crash:before-commit",
		Script: func(s *scenario.Scenario) {
			s.When(scenario.Get("/restart"), scenario.ExpectStatus(http.StatusServiceUnavailable))
		},
	}
	err = d.script(t.Context(), u, &Result{Unit: u, Phases: map[Phase]time.Duration{}})
	if err == nil || !strings.Contains(err.Error(), "observed 0 faultpoint responses, want 1") {
		t.Fatalf("script = %v, want missing faultpoint failure", err)
	}
	if calls != 0 {
		t.Fatalf("restart calls = %d, want 0", calls)
	}
}

func TestEnterStage_dropsAStageAfterTheFlowIsCancelled(t *testing.T) {
	t.Parallel()
	ctx, cancel := context.WithCancel(t.Context())
	cancel()
	enterStage(ctx, make(chan scenario.Stage), scenario.StageGiven)
}

func plantedUnit(outcome string, script flows.Script) Unit {
	return Unit{
		Flow: tools.Flow{
			ID: "99", Trigger: "GET /slow", Outcomes: []tools.Outcome{tools.Outcome(outcome)},
			Events: []string{"system.pinged"}, Consumers: []string{"system.echo"},
		},
		Outcome: tools.Outcome(outcome), Script: script,
	}
}

func slowAPI(t *testing.T) (string, <-chan struct{}) {
	t.Helper()
	arrived, once := make(chan struct{}), sync.Once{}
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) {
		once.Do(func() { close(arrived) })
		<-r.Context().Done()
	}))
	t.Cleanup(srv.Close)
	return srv.URL, arrived
}

func advancing(clk *testkit.Clock, step time.Duration, run func()) {
	done := make(chan struct{})
	var g errgroup.Group
	g.Go(func() error {
		tick := time.NewTicker(time.Millisecond)
		defer tick.Stop()
		for {
			select {
			case <-done:
				return nil
			case <-tick.C:
				clk.Advance(step)
			}
		}
	})
	run()
	close(done)
	_ = g.Wait()
}

func runPastConverge(t *testing.T, d *driver, u Unit) *Result {
	t.Helper()
	clk := fakeClock()
	made := make(chan time.Duration, 16)
	clk.NotifyTickers(made)
	d.clock = clk
	done := make(chan struct{})
	var eg errgroup.Group
	eg.Go(func() error {
		for {
			select {
			case every := <-made:
				if every == logPollEvery {
					clk.Advance(d.budget.Converge)
					return nil
				}
			case <-done:
				return nil
			}
		}
	})
	res := d.run(t.Context(), u)
	close(done)
	_ = eg.Wait()
	return res
}

func fakeClock() *testkit.Clock { return testkit.NewClock(time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC)) }

func TestDriver_aPlantedFlowThatSleepsPastItsBudgetFailsNamingTheFlowPhase(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		script flows.Script
		phase  Phase
		budget func(*Budget)
	}{
		{
			"flow", func(s *scenario.Scenario) { s.Given(scenario.Anonymous()).When(scenario.Get("/slow")) }, PhaseFlow,
			func(b *Budget) { b.Seed, b.Flow = time.Hour, 300*time.Millisecond },
		},
		{
			"seed", func(s *scenario.Scenario) { s.Given(scenario.Get("/slow")) }, PhaseSeed,
			func(b *Budget) { b.Seed, b.Flow = 200*time.Millisecond, time.Hour },
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			api, _ := slowAPI(t)
			env := unservedEnv(api)
			budget := DefaultBudget()
			tc.budget(&budget)
			d, err := newDriver(env, budget)
			if err != nil {
				t.Fatal(err)
			}
			clk := fakeClock()
			d.clock = clk
			var res *Result
			advancing(clk, 10*time.Millisecond, func() { res = d.run(t.Context(), plantedUnit("ok", tc.script)) })
			want := "over budget: flow 99 ok " + string(tc.phase) + " took longer than"
			if res.Pass() || res.Over == nil || res.Over.Phase != tc.phase || !strings.Contains(res.Failure, want) {
				t.Fatalf("result = %+v, want %q", res, want)
			}
		})
	}
}

func TestRunAll_runsPollerUnitsAfterRouteUnitsFinish(t *testing.T) {
	t.Parallel()
	var mu sync.Mutex
	var order []string
	mark := func(name string) {
		mu.Lock()
		order = append(order, name)
		mu.Unlock()
	}
	route := func(name string) Unit {
		return Unit{
			Flow: tools.Flow{ID: name, Trigger: "GET /healthz"},
			Script: func(*scenario.Scenario) {
				mark(name + "-start")
				mark(name + "-end")
			},
		}
	}
	poller := Unit{
		Flow:   tools.Flow{ID: "18", Trigger: "poller:market.prices"},
		Script: func(*scenario.Scenario) { mark("poller-start") },
	}
	env := servedEnv(t)
	budget := DefaultBudget()
	budget.Converge = 30 * time.Millisecond
	d, err := newDriver(env, budget)
	if err != nil {
		t.Fatal(err)
	}
	if results := d.runAll(t.Context(), []Unit{route("a"), poller, route("b")}, 2); len(results) != 3 {
		t.Fatalf("results = %d, want 3", len(results))
	}
	mu.Lock()
	defer mu.Unlock()
	ended := map[string]bool{}
	for _, name := range order {
		if name == "poller-start" {
			if !ended["a-end"] || !ended["b-end"] {
				t.Fatalf("order = %q, want both route scripts to finish before the poller starts", order)
			}
			return
		}
		if strings.HasSuffix(name, "-end") {
			ended[name] = true
		}
	}
	t.Fatalf("order = %q, want the poller to start", order)
}

func TestDriver_stopsAFlowWhenTheRunIsCancelled(t *testing.T) {
	t.Parallel()
	api, arrived := slowAPI(t)
	d, err := newDriver(unservedEnv(api), DefaultBudget())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancelCause(t.Context())
	defer cancel(nil)
	var g errgroup.Group
	g.Go(func() error {
		<-arrived
		cancel(&OverBudgetError{Phase: PhaseTotal, Budget: time.Second})
		return nil
	})
	defer func() { _ = g.Wait() }()
	res := d.run(ctx, plantedUnit("ok", func(s *scenario.Scenario) { s.When(scenario.Get("/slow")) }))
	if res.Over == nil || res.Over.Phase != PhaseTotal {
		t.Fatalf("result = %+v, want the total budget named", res)
	}
}

func connTracker(t *testing.T) (string, func() []http.ConnState) {
	t.Helper()
	var mu sync.Mutex
	conns := map[net.Conn]http.ConnState{}
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	srv.Config.ConnState = func(c net.Conn, st http.ConnState) {
		mu.Lock()
		defer mu.Unlock()
		conns[c] = st
	}
	srv.Start()
	t.Cleanup(srv.Close)
	return srv.URL, func() []http.ConnState {
		mu.Lock()
		defer mu.Unlock()
		var open []http.ConnState
		for _, st := range conns {
			if st != http.StateClosed && st != http.StateHijacked {
				open = append(open, st)
			}
		}
		return open
	}
}

func TestDriver_aFinishedFlowLeavesNoConnectionOpenSoTheAPIShutsDownAtOnce(t *testing.T) {
	t.Parallel()
	env := servedEnv(t)
	var open func() []http.ConnState
	env.API, open = connTracker(t)
	budget := DefaultBudget()
	budget.Converge = virtualConverge
	d, err := newDriver(env, budget)
	if err != nil {
		t.Fatal(err)
	}
	res := runPastConverge(t, d, plantedUnit("ok", func(s *scenario.Scenario) {
		s.Given(scenario.Anonymous()).When(scenario.Get("/slow"), scenario.ExpectStatus(http.StatusOK))
	}))
	if len(res.Exchanges) != 1 || res.Exchanges[0].Status != http.StatusOK {
		t.Fatalf("exchanges = %+v, want one GET /slow answered 200", res.Exchanges)
	}
	testkit.Eventually(t, func() bool { return len(open()) == 0 }, time.Second)
}

func TestDriver_convergenceTimesOutNamingTheStuckConsumer(t *testing.T) {
	t.Parallel()
	env := servedEnv(t)
	ghost := env.Consumers[0].Handlers[0]
	ghost.Name = "ghost.echo"
	env.Consumers = append(env.Consumers, bus.Consumer{Durable: "ghost", Handlers: []bus.HandlerSpec{ghost}})
	budget := DefaultBudget()
	budget.Seed, budget.Flow, budget.Converge = time.Hour, time.Hour, time.Second
	d, err := newDriver(env, budget)
	if err != nil {
		t.Fatal(err)
	}
	u := flow00(t, Target{Flow: "00", Outcome: "ok"})[0]
	u.Flow.Consumers = []string{"ghost"}
	u.Script = func(s *scenario.Scenario) {
		s.Given(scenario.AsUser("alice")).When(scenario.Post("/v1/system/pings", `{"note":"hi"}`))
	}
	clk := fakeClock()
	d.clock = clk
	var res *Result
	advancing(clk, pollEvery, func() { res = d.run(t.Context(), u) })
	want := "consumer ghost handler ghost.echo has not handled event"
	if res.Over == nil || res.Over.Phase != PhaseConverge || !strings.Contains(res.Failure, want) {
		t.Fatalf("result = %+v, want %q", res, want)
	}
}

func TestDriver_reportsInvariantFailures(t *testing.T) {
	t.Parallel()
	served := servedEnv(t)
	post := func(s *scenario.Scenario) {
		s.Given(scenario.AsUser("alice")).When(scenario.Post("/v1/system/pings", `{"note":"hi"}`))
	}
	for _, tc := range []struct {
		name    string
		outcome string
		script  flows.Script
		edit    func(*Env, *Unit)
		want    string
	}{
		{
			"no trigger", "ok", func(s *scenario.Scenario) { s.When(scenario.Get("/healthz")) }, nil,
			"no POST /v1/system/pings request was sent",
		},
		{"error status", "ok", func(s *scenario.Scenario) {
			s.When(scenario.Anonymous(), scenario.Post("/v1/system/pings", `{}`))
		}, nil, "answered 401, want a 2xx for outcome ok"},
		{"wrong code", "InvalidInput", post, nil, `answered 201 code "", want 400 code "invalid_input"`},
		{"missing logs", "ok", post, func(e *Env, _ *Unit) { e.Logs = &Logs{} }, "no http.request log line"},
		{"missing attr", "ok", post, func(e *Env, u *Unit) {
			logs, script := &Logs{}, u.Script
			e.Logs = logs
			u.Script = func(s *scenario.Scenario) {
				script(s)
				logs.add("api", `{"msg":"http.request","method":"POST","route":"/v1/system/pings"}`)
			}
		}, `http.request log line from api lacks required attr "status"`},
		{"line before the script", "ok", post, func(e *Env, _ *Unit) {
			e.Logs = &Logs{}
			e.Logs.add("api",
				`{"msg":"http.request","method":"POST","route":"/v1/system/pings","status":201,"duration_ms":1}`)
		}, "no http.request log line"},
		{"missing consumer", "ok", post, func(e *Env, u *Unit) {
			e.Consumers = []bus.Consumer{{Durable: "nobody", Handlers: e.Consumers[0].Handlers}}
			u.Flow.Consumers = []string{"nobody"}
		}, "consumer nobody:"},
		{
			"failed script", "ok", func(s *scenario.Scenario) { s.Then(scenario.ExpectStatus(http.StatusOK)) }, nil,
			"scenario: no request has been sent",
		},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := served
			u := flow00(t, Target{Flow: "00", Outcome: "ok"})[0]
			u.Outcome, u.Script = tools.Outcome(tc.outcome), tc.script
			if tc.edit != nil {
				tc.edit(&env, &u)
			}
			budget := DefaultBudget()
			budget.Seed, budget.Flow, budget.Converge = time.Minute, time.Minute, virtualConverge
			d, err := newDriver(env, budget)
			if err != nil {
				t.Fatal(err)
			}
			if res := runPastConverge(t, d, u); res.Pass() || !strings.Contains(res.Failure, tc.want) {
				t.Fatalf("result = %+v, want %q", res, tc.want)
			}
		})
	}
}

func TestVerifyUnits_failsOnDeadLettersInternalErrorsAndLedgerChecks(t *testing.T) {
	t.Parallel()
	env := servedEnv(t)
	ns := strings.TrimSuffix(env.DeadLetter, "_"+bus.StreamDeadLetter)
	if _, err := env.JS.Publish(t.Context(), ns+".deadletter.system_echo", []byte("{}")); err != nil {
		t.Fatal(err)
	}
	env.Logs.add("worker", `{"msg":"bus.dispatched","code":"internal"}`)
	cfg, out := driveConfig(DefaultBudget())
	cfg.Ledger = []LedgerCheck{{Name: "cabal", Check: func(context.Context, *pgxpool.Pool) error {
		return errors.New("balance off by 1")
	}}}
	err := verifyUnits(
		t.Context(),
		cfg,
		env,
		newReport(t.Context(), Target{}, flow00(t, Target{Flow: "00", Outcome: "Unauthorized"})),
		1,
	)
	if !errors.Is(err, errFailed) {
		t.Fatalf("verifyUnits = %v, want a failure", err)
	}
	for _, want := range []string{
		"PASS flow 00 Unauthorized",
		"FAIL invariant: 1 dead letters in",
		"FAIL invariant: worker logged an internal error",
		"FAIL invariant: ledger cabal: balance off by 1",
	} {
		if !strings.Contains(out.String(), want) {
			t.Errorf("output lacks %q:\n%s", want, out)
		}
	}
}

func TestWithDeadline_firesItsCauseOnTheClockAndCancelsTwiceSafely(t *testing.T) {
	t.Parallel()
	clk := fakeClock()
	cause := errors.New("budget spent")
	ctx, cancel := withDeadline(t.Context(), clk, time.Second, cause)
	defer cancel()
	for ctx.Err() == nil {
		clk.Advance(time.Second)
		runtime.Gosched()
	}
	if got := context.Cause(ctx); !errors.Is(got, cause) {
		t.Fatalf("cause = %v, want %v", got, cause)
	}
	cancel()
	cancel()
}

func TestCancelCause_namesTheCauseOnlyWhenAFailedFlowWasCancelled(t *testing.T) {
	t.Parallel()
	cause, failure := errors.New("run stopped"), errors.New("scenario: GET /slow: context canceled")
	live := t.Context()
	cancelled, cancel := context.WithCancelCause(t.Context())
	cancel(cause)
	for _, tc := range []struct {
		name    string
		stopped bool
		failure error
		want    error
	}{
		{"cancelled and failed", true, failure, cause},
		{"cancelled and passed", true, nil, nil},
		{"live and failed", false, failure, failure},
	} {
		ctx := live
		if tc.stopped {
			ctx = cancelled
		}
		got := cancelCause(ctx, "99 ok", tc.failure)
		if !errors.Is(got, tc.want) || (tc.want == nil) != (got == nil) {
			t.Errorf("%s: cancelCause = %v, want %v", tc.name, got, tc.want)
		}
	}
}

func TestClientIP_givesEachUnitItsOwnStableAddress(t *testing.T) {
	t.Parallel()
	join := Unit{
		Flow: tools.Flow{ID: "03", Commands: []string{"JoinCabal", "RequestAccess"}}, Command: "JoinCabal",
		Outcome: tools.OutcomeOK,
	}
	request := join
	request.Command = "RequestAccess"
	first, again, other := clientIP(join), clientIP(join), clientIP(request)
	if first != again || first == other {
		t.Fatalf("clientIP: join %s then %s, request %s", first, again, other)
	}
	if ip := net.ParseIP(first); ip == nil || !ip.IsPrivate() {
		t.Fatalf("clientIP = %q, want a private address", first)
	}
}
