package verify

import (
	"bytes"
	"context"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"

	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit"
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
	sv := scenario.Serve(t,
		scenario.WithModules(mods...),
		scenario.WithLogs(&lineWriter{line: func(s string) { logs.add("app", s) }}))
	return Env{
		API: sv.URL, TokenKey: sv.TokenKey, Pool: sv.Pool, JS: sv.JS, Events: sv.Events,
		DeadLetter: sv.DeadLetter, Subject: sv.Subject, Consumers: sv.Consumers, Logs: logs,
		Arm: func(context.Context) error { return nil },
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

func plantedUnit(outcome string, script flows.Script) Unit {
	return Unit{
		Flow: tools.Flow{
			ID: "99", Trigger: "GET /slow", Outcomes: []tools.Outcome{tools.Outcome(outcome)},
			Events: []string{"system.pinged"}, Consumers: []string{"system.echo"},
		},
		Outcome: tools.Outcome(outcome), Script: script,
	}
}

func slowAPI(t *testing.T) string {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, r *http.Request) { <-r.Context().Done() }))
	t.Cleanup(srv.Close)
	return srv.URL
}

func TestDriver_aPlantedFlowThatSleepsPastItsBudgetFailsNamingTheFlowPhase(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name   string
		script flows.Script
		phase  Phase
	}{
		{"flow", func(s *scenario.Scenario) { s.Given(scenario.Anonymous()).When(scenario.Get("/slow")) }, PhaseFlow},
		{"seed", func(s *scenario.Scenario) { s.Given(scenario.Get("/slow")) }, PhaseSeed},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			env := servedEnv(t)
			env.API = slowAPI(t)
			budget := DefaultBudget()
			budget.Seed, budget.Flow = 200*time.Millisecond, 300*time.Millisecond
			d, err := newDriver(env, budget)
			if err != nil {
				t.Fatal(err)
			}
			res := d.run(t.Context(), plantedUnit("ok", tc.script))
			want := "over budget: flow 99 ok " + string(tc.phase) + " took longer than"
			if res.Pass() || res.Over == nil || res.Over.Phase != tc.phase || !strings.Contains(res.Failure, want) {
				t.Fatalf("result = %+v, want %q", res, want)
			}
		})
	}
}

func TestDriver_stopsAFlowWhenTheRunIsCancelled(t *testing.T) {
	t.Parallel()
	env := servedEnv(t)
	env.API = slowAPI(t)
	d, err := newDriver(env, DefaultBudget())
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeoutCause(t.Context(), 200*time.Millisecond,
		&OverBudgetError{Phase: PhaseTotal, Budget: time.Second})
	defer cancel()
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
	d, err := newDriver(env, DefaultBudget())
	if err != nil {
		t.Fatal(err)
	}
	res := d.run(t.Context(), plantedUnit("ok", func(s *scenario.Scenario) {
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
	budget.Converge = time.Second
	d, err := newDriver(env, budget)
	if err != nil {
		t.Fatal(err)
	}
	u := flow00(t, Target{Flow: "00", Outcome: "ok"})[0]
	u.Flow.Consumers = append(u.Flow.Consumers, "ghost")
	u.Script = func(s *scenario.Scenario) {
		s.Given(scenario.AsUser("alice")).When(scenario.Post("/v1/system/pings", `{"note":"hi"}`))
	}
	res := d.run(t.Context(), u)
	want := "consumer ghost handler ghost.echo has not handled event"
	if res.Over == nil || res.Over.Phase != PhaseConverge || !strings.Contains(res.Failure, want) {
		t.Fatalf("result = %+v, want %q", res, want)
	}
}

func TestDriver_reportsInvariantFailures(t *testing.T) {
	t.Parallel()
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
			env := servedEnv(t)
			u := flow00(t, Target{Flow: "00", Outcome: "ok"})[0]
			u.Outcome, u.Script = tools.Outcome(tc.outcome), tc.script
			if tc.edit != nil {
				tc.edit(&env, &u)
			}
			d, err := newDriver(env, DefaultBudget())
			if err != nil {
				t.Fatal(err)
			}
			if res := d.run(t.Context(), u); res.Pass() || !strings.Contains(res.Failure, tc.want) {
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
