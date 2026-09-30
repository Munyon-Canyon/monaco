package verify

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"github.com/monaco/monaco/apps/backend/internal/platform/faultpoint"
	"github.com/monaco/monaco/apps/backend/internal/testkit/scenario"
	tools "github.com/monaco/monaco/apps/backend/internal/tools/flows"
)

const pollInterval = "MARKET_PRICE_POLL_INTERVAL"

func TestWorkerEnv_joinsTheVariablesOfTheSelectedFlowsOnceAndRejectsAConflict(t *testing.T) {
	t.Parallel()
	units := []Unit{
		{Flow: tools.Flow{ID: "90"}, Outcome: "ok"},
		{Flow: tools.Flow{ID: "90"}, Outcome: "InvalidInput"},
		{Flow: tools.Flow{ID: "92"}, Outcome: "ok"},
	}
	env := map[string][]string{
		"90": {pollInterval + "=2s", "A=1"},
		"92": {pollInterval + "=2s", "B=2"},
		"93": {pollInterval + "=9s"},
	}
	got, err := workerEnv(units, env)
	if want := []string{pollInterval + "=2s", "A=1", "B=2"}; err != nil || !slices.Equal(got, want) {
		t.Fatalf("workerEnv = %q, %v, want %q", got, err, want)
	}
	env["92"] = []string{pollInterval + "=5s"}
	want := `worker environment: flows 90 and 92 set MARKET_PRICE_POLL_INTERVAL to "2s" and "5s"`
	if _, err := workerEnv(units, env); !errors.Is(err, errWorkerEnv) || err.Error() != want {
		t.Fatalf("workerEnv with a conflict = %v, want %q", err, want)
	}
}

func TestRun_twoFlowsThatSetOneWorkerVariableDifferentlyFailBeforeTheStackStarts(t *testing.T) {
	t.Parallel()
	cfg, stdout, stderr := testConfig(t)
	flows := tools.Header + "\n" +
		"90\tHealth\tsystem\tGET /healthz\tHealth\t\t\tok\tbuilt\tdocs/x.md#health\n" +
		"92\tPrices\tfixture\tpoller:fixture.prices\tTickPrices\t\t\tok\tbuilt\tdocs/x.md#prices\n"
	if err := os.WriteFile(filepath.Join(cfg.Dir, tools.File), []byte(flows), 0o600); err != nil {
		t.Fatal(err)
	}
	cfg.Scripts["F92TickPricesOK"] = func(*scenario.Scenario) {}
	cfg.Env = map[string][]string{"90": {pollInterval + "=2s"}, "92": {pollInterval + "=5s"}}
	want := `monacoctl verify: worker environment: flows 90 and 92 set MARKET_PRICE_POLL_INTERVAL to "2s" and "5s"`
	if code := Run(t.Context(), cfg, Target{}); code != 1 || strings.TrimSpace(stderr.String()) != want ||
		stdout.Len() != 0 {
		t.Fatalf("Run = %d, stdout %q, stderr %q, want 1 and %q", code, stdout, stderr, want)
	}
	if _, err := os.Stat(filepath.Join(filepath.Dir(cfg.Go), "args")); !errors.Is(err, fs.ErrNotExist) {
		t.Fatalf("go build ran before the conflict stopped the run: %v", err)
	}
}

func TestRun_startsTheWorkerWithTheVariablesOfTheSelectedFlows(t *testing.T) {
	t.Parallel()
	for _, tc := range []struct {
		name string
		env  map[string][]string
		code int
		want string
	}{
		{"set", map[string][]string{"90": {pollInterval + "=2s"}}, 0, ""},
		{"unset", nil, 1, "worker exited before boot.listening: exit status 5"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Parallel()
			cfg, stdout, stderr := testConfig(t)
			cfg.Environ = append(cfg.Environ, fakeNeedsEnv+"="+pollInterval)
			cfg.Env = tc.env
			if code := Run(t.Context(), cfg, Target{Flow: "90"}); code != tc.code ||
				!strings.Contains(stderr.String(), tc.want) {
				t.Fatalf("Run = %d, want %d and %q\n%s\n%s", code, tc.code, tc.want, stdout, stderr)
			}
		})
	}
}

func TestStack_aRestartedWorkerKeepsTheFlowsVariables(t *testing.T) {
	t.Parallel()
	o := testOptions(t, "ok")
	o.Faultpoint = string(faultpoint.AfterPublish)
	o.WorkerEnv = []string{pollInterval + "=2s"}
	o.Environ = append(o.Environ, fakeNeedsEnv+"="+pollInterval)
	s, err := Up(t.Context(), o)
	defer func() { _ = s.Down(t.Context()) }()
	if err != nil {
		t.Fatalf("Up: %v", err)
	}
	if err := s.crash(t.Context(), faultpoint.AfterPublish); err != nil {
		t.Fatalf("restart after the crash: %v", err)
	}
	if err := s.arm(t.Context()); err != nil {
		t.Fatalf("restart armed: %v", err)
	}
}
