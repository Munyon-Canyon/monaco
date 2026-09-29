package verify

import (
	"bytes"
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/modules/system"
	"github.com/monaco/monaco/apps/backend/internal/platform/bus"
	"github.com/monaco/monaco/apps/backend/internal/platform/module"
	"github.com/monaco/monaco/apps/backend/internal/testkit/flows"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

func servedEnv(t *testing.T) Env {
	t.Helper()
	logs := &Logs{}
	sv := scenario.Serve(t,
		scenario.WithModules(func(d module.Deps) module.Module { return system.New(d) }),
		scenario.WithLogs(&lineWriter{line: func(s string) { logs.add("app", s) }}))
	return Env{
		API: sv.URL, TokenKey: sv.TokenKey, Pool: sv.Pool, JS: sv.JS, Events: sv.Events,
		DeadLetter: sv.DeadLetter, Consumers: sv.Consumers, Logs: logs,
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
		flow00(t, Target{Flow: "00"}),
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
