package verify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
)

func readEvidence(t *testing.T, path string) Evidence {
	t.Helper()
	body, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var ev Evidence
	if err := json.Unmarshal(body, &ev); err != nil {
		t.Fatal(err)
	}
	return ev
}

func TestReport_writesOneEvidenceFilePerFlowWithWhatTheSystemDid(t *testing.T) {
	t.Parallel()
	env := servedEnv(t)
	cfg, out := driveConfig(DefaultBudget())
	rep := newReport(t.Context(), Target{}, flow00(t, Target{Flow: "00"}))
	if err := verifyUnits(t.Context(), cfg, env, rep, parallelFlows); err != nil {
		t.Fatalf("verifyUnits: %v\n%s", err, out)
	}
	rep.results[0].Exchanges = append(rep.results[0].Exchanges, scenario.Exchange{
		Method: "POST", Path: "/x", Status: 201, Took: time.Millisecond,
		Request: []byte(`{"access_token":"t0k","items":[{"client_secret":"s3c","n":1}]}`), Response: []byte("not json"),
	})
	rep.phases[PhaseStack] = 1500 * time.Millisecond
	dir := t.TempDir()
	if err := rep.write(t.Context(), dir, nil); err != nil {
		t.Fatal(err)
	}
	ev := readEvidence(t, filepath.Join(dir, ".verify", "00.json"))
	if ev.Result != resultPass || len(ev.Outcomes) != 3 || ev.Commit != "" || !ev.Dirty || ev.Host.CPUs == 0 ||
		ev.PhasesMS[PhaseStack] != 1500 || ev.LatencyMS.P95 < ev.LatencyMS.P50 {
		t.Fatalf("evidence = %+v", ev)
	}
	checkConsumers(t, ev.Consumers)
	checkOKOutcome(t, ev.Outcomes[0])
}

func checkConsumers(t *testing.T, consumers []ConsumerEvidence) {
	t.Helper()
	if len(consumers) != 1 || consumers[0].Durable != "system_echo" || consumers[0].AckFloor == 0 {
		t.Fatalf("consumers = %+v", consumers)
	}
}

func checkOKOutcome(t *testing.T, ok OutcomeEvidence) {
	t.Helper()
	if ok.Outcome != "ok" || len(ok.Events) != 1 || strings.Join(ok.Events[0].Deliveries, ",") != "system.echo" ||
		!ok.Events[0].Published || len(ok.LogLines) < 3 || ok.PhasesMS[PhaseFlow] == 0 {
		t.Fatalf("ok outcome = %+v", ok)
	}
	planted := ok.Exchanges[len(ok.Exchanges)-1]
	var request bytes.Buffer
	if err := json.Compact(&request, planted.Request); err != nil ||
		request.String() != `{"access_token":"***","items":[{"client_secret":"***","n":1}]}` ||
		strings.Trim(string(planted.Response), "nul") != "" {
		t.Fatalf("planted exchange = %s / %s, want tokens redacted", planted.Request, planted.Response)
	}
}

func TestReport_marksOverBudgetAndFailuresAndNamesThePhase(t *testing.T) {
	t.Parallel()
	units := []Unit{plantedUnit("ok", nil), plantedUnit("InvalidInput", nil)}
	for _, tc := range []struct {
		name    string
		results []*Result
		runErr  error
		target  Target
		file    string
		result  string
		phase   Phase
	}{
		{"flow over budget", []*Result{
			{Unit: units[0], Failure: "slow", Over: &OverBudgetError{Phase: PhaseFlow, Flow: "99 ok"}},
			{Unit: units[1], Failure: "wrong code"},
		}, nil, Target{}, "99.json", resultOverBudget, PhaseFlow},
		{
			"failure then over",
			[]*Result{{Unit: units[0], Failure: "wrong code"}},
			&OverBudgetError{Phase: PhaseTeardown},
			Target{CrashAt: "after-publish"},
			"99-crash-after-publish.json", resultOverBudget, PhaseTeardown,
		},
		{"run failure", []*Result{{Unit: units[0]}}, errors.New("stack broke"), Target{}, "99.json", resultFail, ""},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			rep := newReport(t.Context(), tc.target, units[:len(tc.results)])
			rep.results = tc.results
			dir := t.TempDir()
			if err := rep.write(t.Context(), dir, tc.runErr); err != nil {
				t.Fatal(err)
			}
			ev := readEvidence(t, filepath.Join(dir, ".verify", tc.file))
			if ev.Result != tc.result || ev.Phase != tc.phase || ev.Error == "" {
				t.Fatalf("evidence = %+v, want %s in %q", ev, tc.result, tc.phase)
			}
		})
	}
}

func TestReport_writeFailsWhenTheEvidenceDirIsAFile(t *testing.T) {
	t.Parallel()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".verify"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	rep := newReport(t.Context(), Target{}, []Unit{plantedUnit("ok", nil)})
	if err := rep.write(t.Context(), dir, nil); err == nil || !strings.Contains(err.Error(), "write evidence") {
		t.Fatalf("write = %v", err)
	}
}

func TestGitState_readsTheCommitOfARepo(t *testing.T) {
	t.Parallel()
	if commit, _ := gitState(t.Context(), backendDir(t)); len(commit) != 40 {
		t.Fatalf("commit = %q, want a full sha", commit)
	}
}

func TestVerifyUnits_namesTheOverBudgetPhaseAndStopsOnCollectFailures(t *testing.T) {
	t.Parallel()
	served := servedEnv(t)
	env := served
	env.API, _ = slowAPI(t)
	budget := DefaultBudget()
	budget.Seed, budget.Flow = time.Hour, 200*time.Millisecond
	cfg, _ := driveConfig(budget)
	clk := fakeClock()
	cfg.Clock = clk
	slow := plantedUnit("ok", func(s *scenario.Scenario) { s.When(scenario.Get("/slow")) })
	var err error
	advancing(clk, 10*time.Millisecond, func() {
		err = verifyUnits(t.Context(), cfg, env, newReport(t.Context(), Target{}, []Unit{slow}), 1)
	})
	if !strings.Contains(err.Error(), "over budget: flow 99 ok flow took longer than 200ms") {
		t.Fatalf("verifyUnits = %v, want the phase named", err)
	}
	cancelled, cancel := context.WithCancel(t.Context())
	cancel()
	rep := newReport(t.Context(), Target{}, flow00(t, Target{Flow: "00", Outcome: "ok"}))
	if err := verifyUnits(cancelled, cfg, served, rep, 1); err == nil ||
		!strings.Contains(err.Error(), "read deliveries") {
		t.Fatalf("verifyUnits on a cancelled context = %v", err)
	}
	d, err := newDriver(env, budget)
	if err != nil {
		t.Fatal(err)
	}
	missing := &report{results: []*Result{{Unit: plantedUnit("ok", nil)}}}
	d.env.Consumers[0].Durable = "gone"
	missing.results[0].Unit.Flow.Consumers = []string{"gone"}
	if err := d.collect(t.Context(), missing); err == nil || !strings.Contains(err.Error(), "consumer gone") {
		t.Fatalf("collect = %v", err)
	}
}
